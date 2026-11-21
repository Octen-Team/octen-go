package octen

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const okBody = `{"code":0,"msg":"ok","request_id":"rid-ok","data":{"query":"q","results":[]}}`

// fakeServer counts requests and hands each one its 1-based arrival number.
type fakeServer struct {
	*httptest.Server
	hits atomic.Int32
}

func newFakeServer(t *testing.T, h func(w http.ResponseWriter, r *http.Request, n int)) *fakeServer {
	t.Helper()
	f := &fakeServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		h(w, r, int(f.hits.Add(1)))
	}))
	t.Cleanup(f.Close)
	return f
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, okBody)
}

// sleepRecorder records requested backoff waits without actually waiting.
type sleepRecorder struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (s *sleepRecorder) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.waits = append(s.waits, d)
	s.mu.Unlock()
	return ctx.Err()
}

func (s *sleepRecorder) got() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.waits...)
}

func testTransport(baseURL string, client *http.Client) (*transport, *sleepRecorder) {
	tr := newTransport(baseURL, "test-key", nil, client, 3)
	rec := &sleepRecorder{}
	tr.sleep = rec.sleep
	tr.rnd = func() float64 { return 0 }
	return tr, rec
}

// postBody returns the body of a successful envelope, or the error.
func postBody(tr *transport, ctx context.Context, body []byte, timeout time.Duration, extra http.Header) ([]byte, error) {
	res, err := tr.postResponse(ctx, "/search", body, timeout, extra)
	if err != nil {
		return nil, err
	}
	return res.body, nil
}

func post(tr *transport, timeout time.Duration) ([]byte, error) {
	return postBody(tr, context.Background(), []byte(`{"query":"q"}`), timeout, nil)
}

// stallUntilGone blocks until the client goes away, bounded so a broken client cannot hang Close.
func stallUntilGone(r *http.Request, limit time.Duration) {
	select {
	case <-r.Context().Done():
	case <-time.After(limit):
	}
}

func TestRetriesEachRetryableStatus(t *testing.T) {
	for _, status := range []int{408, 429, 500, 502, 503, 504} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
				if n <= 3 {
					w.WriteHeader(status)
					_, _ = io.WriteString(w, "try again")
					return
				}
				writeOK(w)
			})
			tr, rec := testTransport(s.URL, nil)
			body, err := post(tr, 5*time.Second)
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			if string(body) != okBody {
				t.Fatalf("body = %q, want %q", body, okBody)
			}
			if got := s.hits.Load(); got != 4 {
				t.Fatalf("server saw %d requests, want 4", got)
			}
			want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
			if got := rec.got(); !reflect.DeepEqual(got, want) {
				t.Fatalf("backoff waits = %v, want %v", got, want)
			}
		})
	}
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.WriteHeader(503)
		_, _ = io.WriteString(w, "unavailable")
	})
	tr, rec := testTransport(s.URL, nil)
	_, err := post(tr, 5*time.Second)
	if !errors.Is(err, ErrServer) {
		t.Fatalf("err = %v, want ErrServer", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 503 || string(apiErr.Body) != "unavailable" {
		t.Fatalf("err = %#v, want *APIError 503 with body", err)
	}
	if got := s.hits.Load(); got != 4 {
		t.Fatalf("server saw %d requests, want 4", got)
	}
	if got := len(rec.got()); got != 3 {
		t.Fatalf("slept %d times, want 3", got)
	}
}

func TestZeroMaxRetriesSendsOnce(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { w.WriteHeader(500) })
	tr, rec := testTransport(s.URL, nil)
	tr.maxRetries = 0
	if _, err := post(tr, 5*time.Second); !errors.Is(err, ErrServer) {
		t.Fatalf("err = %v, want ErrServer", err)
	}
	if got := s.hits.Load(); got != 1 || len(rec.got()) != 0 {
		t.Fatalf("requests = %d, sleeps = %d; want 1, 0", got, len(rec.got()))
	}
}

func TestRetryAfterSecondsAndDate(t *testing.T) {
	t.Run("seconds", func(t *testing.T) {
		s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
			if n == 1 {
				w.Header().Set("Retry-After", "7")
				w.WriteHeader(429)
				return
			}
			writeOK(w)
		})
		tr, rec := testTransport(s.URL, nil)
		if _, err := post(tr, 5*time.Second); err != nil {
			t.Fatalf("post: %v", err)
		}
		if got := rec.got(); !reflect.DeepEqual(got, []time.Duration{7 * time.Second}) {
			t.Fatalf("waits = %v, want [7s]", got)
		}
	})
	t.Run("zero", func(t *testing.T) {
		s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
			if n == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(503)
				return
			}
			writeOK(w)
		})
		tr, rec := testTransport(s.URL, nil)
		if _, err := post(tr, 5*time.Second); err != nil {
			t.Fatalf("post: %v", err)
		}
		if got := rec.got(); !reflect.DeepEqual(got, []time.Duration{0}) {
			t.Fatalf("waits = %v, want [0s]", got)
		}
	})
	t.Run("date", func(t *testing.T) {
		s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
			if n == 1 {
				w.Header().Set("Retry-After", time.Now().Add(20*time.Second).UTC().Format(http.TimeFormat))
				w.WriteHeader(503)
				return
			}
			writeOK(w)
		})
		tr, rec := testTransport(s.URL, nil)
		if _, err := post(tr, 5*time.Second); err != nil {
			t.Fatalf("post: %v", err)
		}
		got := rec.got()
		// HTTP dates have whole-second precision, so allow up to two seconds of truncation.
		if len(got) != 1 || got[0] <= 17*time.Second || got[0] > 20*time.Second {
			t.Fatalf("waits = %v, want one wait in (17s, 20s]", got)
		}
	})
	t.Run("ignored for 500", func(t *testing.T) {
		s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
			if n == 1 {
				w.Header().Set("Retry-After", "30")
				w.WriteHeader(500)
				return
			}
			writeOK(w)
		})
		tr, rec := testTransport(s.URL, nil)
		if _, err := post(tr, 5*time.Second); err != nil {
			t.Fatalf("post: %v", err)
		}
		if got := rec.got(); !reflect.DeepEqual(got, []time.Duration{time.Second}) {
			t.Fatalf("waits = %v, want [1s]", got)
		}
	})
}

func TestRetryAfterAbove60GivesUp(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.Header().Set("Retry-After", "120")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		_, _ = io.WriteString(w, `{"code":429,"msg":"rate limited","request_id":"rid-429"}`)
	})
	tr, rec := testTransport(s.URL, nil)
	_, err := post(tr, 5*time.Second)
	if !errors.Is(err, ErrRateLimit) {
		t.Fatalf("err = %v, want ErrRateLimit", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RetryAfter == nil || *apiErr.RetryAfter != 120*time.Second {
		t.Fatalf("err = %#v, want RetryAfter 120s", err)
	}
	if apiErr.RequestID != "rid-429" || apiErr.Message != "rate limited" {
		t.Fatalf("RequestID/Message = %q/%q", apiErr.RequestID, apiErr.Message)
	}
	if got := s.hits.Load(); got != 1 || len(rec.got()) != 0 {
		t.Fatalf("requests = %d, sleeps = %d; want 1, 0", got, len(rec.got()))
	}
}

func TestTimeoutNotRetried(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		stallUntilGone(r, time.Second)
		writeOK(w)
	})
	tr, rec := testTransport(s.URL, nil)
	start := time.Now()
	_, err := post(tr, 200*time.Millisecond)
	elapsed := time.Since(start)
	var te *TimeoutError
	if !errors.As(err, &te) || te.Timeout != 200*time.Millisecond {
		t.Fatalf("err = %v, want *TimeoutError after 200ms", err)
	}
	if err.Error() != "octen: request timed out after 200ms" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err should unwrap to the attempt's deadline error: %v / %#v", err, te.Err)
	}
	if got := s.hits.Load(); got != 1 || len(rec.got()) != 0 {
		t.Fatalf("requests = %d, sleeps = %d; want 1, 0", got, len(rec.got()))
	}
	if elapsed > 900*time.Millisecond {
		t.Fatalf("took %v, want about 200ms", elapsed)
	}
}

func TestTimeoutCoversBodyRead(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"code":0,`)
		w.(http.Flusher).Flush()
		stallUntilGone(r, 2*time.Second)
	})
	tr, rec := testTransport(s.URL, nil)
	start := time.Now()
	_, err := post(tr, 200*time.Millisecond)
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TimeoutError", err)
	}
	if got := s.hits.Load(); got != 1 || len(rec.got()) != 0 {
		t.Fatalf("requests = %d, sleeps = %d; want 1, 0", got, len(rec.got()))
	}
	if elapsed := time.Since(start); elapsed > 900*time.Millisecond {
		t.Fatalf("took %v, want about 200ms", elapsed)
	}
}

func TestCanceledContextSendsNothing(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { writeOK(w) })
	tr, _ := testTransport(s.URL, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := postBody(tr, ctx, []byte(`{}`), 5*time.Second, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if !strings.HasPrefix(err.Error(), "octen: ") {
		t.Fatalf("Error() = %q, want octen: prefix", err.Error())
	}
	if got := s.hits.Load(); got != 0 {
		t.Fatalf("server saw %d requests, want 0", got)
	}
}

func TestCallerDeadlineIsNotSDKTimeout(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		stallUntilGone(r, time.Second)
		writeOK(w)
	})
	tr, rec := testTransport(s.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := postBody(tr, ctx, []byte(`{}`), 5*time.Second, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	var te *TimeoutError
	if errors.As(err, &te) {
		t.Fatalf("caller deadline reported as SDK timeout: %v", err)
	}
	if got := s.hits.Load(); got != 1 || len(rec.got()) != 0 {
		t.Fatalf("requests = %d, sleeps = %d; want 1, 0", got, len(rec.got()))
	}
}

func TestCancelDuringBackoff(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { w.WriteHeader(503) })
	tr, _ := testTransport(s.URL, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sleeps := 0
	tr.sleep = func(ctx context.Context, d time.Duration) error {
		sleeps++
		cancel() // the caller gives up while the SDK is waiting
		<-ctx.Done()
		return ctx.Err()
	}
	_, err := postBody(tr, ctx, []byte(`{}`), 5*time.Second, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := s.hits.Load(); got != 1 || sleeps != 1 {
		t.Fatalf("requests = %d, sleeps = %d; want 1, 1", got, sleeps)
	}
}

func TestCancelWhileSleepIgnoresIt(t *testing.T) {
	// A sleep that ignores cancellation must still not lead to another request.
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { w.WriteHeader(503) })
	tr, _ := testTransport(s.URL, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr.sleep = func(context.Context, time.Duration) error { cancel(); return nil }
	if _, err := postBody(tr, ctx, []byte(`{}`), 5*time.Second, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := s.hits.Load(); got != 1 {
		t.Fatalf("server saw %d requests, want 1", got)
	}
}

func TestDefaultSleep(t *testing.T) {
	if err := sleepCtx(context.Background(), 0); err != nil {
		t.Fatalf("zero wait: %v", err)
	}
	if err := sleepCtx(context.Background(), 10*time.Millisecond); err != nil {
		t.Fatalf("short wait: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := sleepCtx(ctx, time.Hour); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("sleep ignored cancellation for %v", elapsed)
	}
}

func closedPortURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return "http://" + addr
}

func TestConnectionRefusedIsRetried(t *testing.T) {
	var dials atomic.Int32
	d := &net.Dialer{}
	rt := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		dials.Add(1)
		return d.DialContext(ctx, network, addr)
	}}
	defer rt.CloseIdleConnections()
	tr, rec := testTransport(closedPortURL(t), &http.Client{Transport: rt})
	_, err := post(tr, 5*time.Second)
	var ce *ConnectionError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *ConnectionError", err)
	}
	if !strings.HasPrefix(err.Error(), "octen: connection error: ") || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("Error() = %q", err.Error())
	}
	if got := dials.Load(); got != 4 {
		t.Fatalf("dialed %d times, want 4", got)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if got := rec.got(); !reflect.DeepEqual(got, want) {
		t.Fatalf("backoff waits = %v, want %v", got, want)
	}
}

func TestDropMidResponseNotRetried(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Type: appli")
		_ = conn.Close()
	})
	tr, rec := testTransport(s.URL, nil)
	_, err := post(tr, 5*time.Second)
	var ce *ConnectionError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *ConnectionError", err)
	}
	if !strings.HasPrefix(err.Error(), "octen: connection error: ") {
		t.Fatalf("Error() = %q", err.Error())
	}
	if got := s.hits.Load(); got != 1 || len(rec.got()) != 0 {
		t.Fatalf("requests = %d, sleeps = %d; want 1, 0", got, len(rec.got()))
	}
}

func TestRedirectNotFollowed(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		status := status
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			target := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { writeOK(w) })
			origin := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
				w.Header().Set("Location", target.URL+"/search")
				w.Header().Set("X-Request-Id", "rid-redirect")
				w.WriteHeader(status)
			})
			tr, rec := testTransport(origin.URL, nil)
			_, err := post(tr, 5*time.Second)
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want *APIError", err)
			}
			want := "HTTP " + strconv.Itoa(status) + " redirect to " + target.URL + "/search; redirects are not followed, check baseURL"
			if apiErr.Message != want || apiErr.StatusCode != status || apiErr.RequestID != "rid-redirect" {
				t.Fatalf("got %d %q (rid %q), want %d %q", apiErr.StatusCode, apiErr.Message, apiErr.RequestID, status, want)
			}
			if origin.hits.Load() != 1 || target.hits.Load() != 0 || len(rec.got()) != 0 {
				t.Fatalf("origin = %d, target = %d, sleeps = %d; want 1, 0, 0", origin.hits.Load(), target.hits.Load(), len(rec.got()))
			}
		})
	}

	t.Run("no location", func(t *testing.T) {
		s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { w.WriteHeader(307) })
		tr, _ := testTransport(s.URL, nil)
		_, err := post(tr, 5*time.Second)
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Message != "HTTP 307 redirect; redirects are not followed, check baseURL" {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRetryReplaysBodyAndHeaders(t *testing.T) {
	var mu sync.Mutex
	var seen []seenRequest
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, seenRequest{body: string(b), header: r.Header.Clone()})
		mu.Unlock()
		if hits.Add(1) <= 2 {
			w.WriteHeader(503)
			return
		}
		writeOK(w)
	}))
	defer s.Close()
	tr, _ := testTransport(s.URL, nil)
	const body = `{"query":"replay me"}`
	if _, err := postBody(tr, context.Background(), []byte(body), 5*time.Second, nil); err != nil {
		t.Fatalf("post: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 {
		t.Fatalf("server saw %d requests, want 3", len(seen))
	}
	for i, r := range seen {
		if r.body != body || r.header.Get("X-Api-Key") != "test-key" || r.header.Get("User-Agent") != "octen-go-sdk/"+Version {
			t.Errorf("attempt %d: body %q, x-api-key %q, User-Agent %q", i+1, r.body, r.header.Get("X-Api-Key"), r.header.Get("User-Agent"))
		}
	}
}

// Both attempts take 200 ms, more than the 300 ms timeout together but not each.
func TestEachAttemptGetsFullTimeout(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		time.Sleep(200 * time.Millisecond)
		if n == 1 {
			w.WriteHeader(503)
			return
		}
		writeOK(w)
	})
	tr, _ := testTransport(s.URL, nil)
	if _, err := post(tr, 300*time.Millisecond); err != nil {
		t.Fatalf("err = %v, want success on the second attempt", err)
	}
	if got := s.hits.Load(); got != 2 {
		t.Fatalf("server saw %d requests, want 2", got)
	}
}

type countingRoundTripper struct {
	n  atomic.Int32
	rt http.RoundTripper
}

func (c *countingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	c.n.Add(1)
	return c.rt.RoundTrip(r)
}

func TestCallerHTTPClientNotMutated(t *testing.T) {
	target := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) { writeOK(w) })
	origin := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	})
	followed := 0
	checkRedirect := func(*http.Request, []*http.Request) error { followed++; return nil }
	rt := &countingRoundTripper{rt: http.DefaultTransport}
	caller := &http.Client{Transport: rt, CheckRedirect: checkRedirect, Timeout: 7 * time.Second}

	tr, _ := testTransport(origin.URL, caller)
	if tr.client == caller {
		t.Fatal("transport uses the caller's client instead of a copy")
	}
	if reflect.ValueOf(caller.CheckRedirect).Pointer() != reflect.ValueOf(checkRedirect).Pointer() || caller.Timeout != 7*time.Second {
		t.Fatal("caller's client was modified")
	}
	if tr.client.Timeout != 7*time.Second || tr.client.Transport != rt {
		t.Fatal("copy lost the caller's Timeout or Transport")
	}
	if _, err := post(tr, 5*time.Second); err == nil {
		t.Fatal("redirect was followed")
	}
	if followed != 0 || target.hits.Load() != 0 || rt.n.Load() != 1 {
		t.Fatalf("followed = %d, target = %d, round trips = %d; want 0, 0, 1", followed, target.hits.Load(), rt.n.Load())
	}
	// The caller's own client still follows redirects as configured.
	resp, err := caller.Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if followed != 1 || target.hits.Load() != 1 {
		t.Fatalf("caller client: followed = %d, target = %d; want 1, 1", followed, target.hits.Load())
	}
}

func TestNilClientUsesDefaults(t *testing.T) {
	tr := newTransport("http://example.invalid", "k", nil, nil, 3)
	if tr.client == nil || tr.client == http.DefaultClient || tr.client.CheckRedirect == nil {
		t.Fatalf("client = %#v, want a private copy with CheckRedirect set", tr.client)
	}
}

func TestRequestHeaders(t *testing.T) {
	type seen struct {
		method, path, body string
		h                  http.Header
	}
	got := make(chan seen, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- seen{r.Method, r.URL.Path, string(b), r.Header.Clone()}
		writeOK(w)
	}))
	defer s.Close()
	tr := newTransport(s.URL+"/", "test-key", http.Header{"X-Client": {"c"}, "x-api-key": {"wrong"}}, nil, 3)
	extra := http.Header{"X-Extra": {"e"}, "X-Api-Key": {"wrong-too"}}
	if _, err := postBody(tr, context.Background(), []byte(`{"query":"q"}`), 5*time.Second, extra); err != nil {
		t.Fatalf("post: %v", err)
	}
	r := <-got
	if r.method != http.MethodPost || r.path != "/search" || r.body != `{"query":"q"}` {
		t.Fatalf("method/path/body = %s %s %s", r.method, r.path, r.body)
	}
	want := map[string]string{
		"X-Api-Key":    "test-key",
		"User-Agent":   "octen-go-sdk/0.2.0",
		"Content-Type": "application/json",
		"Accept":       "application/json",
		"X-Client":     "c",
		"X-Extra":      "e",
	}
	for k, v := range want {
		if vs := r.h.Values(k); len(vs) != 1 || vs[0] != v {
			t.Errorf("%s = %q, want [%q]", k, vs, v)
		}
	}
}

func TestHTTP200NonZeroCodeNotRetried(t *testing.T) {
	s := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":5001,"msg":"quota exhausted"}`)
	})
	tr, rec := testTransport(s.URL, nil)
	_, err := post(tr, 5*time.Second)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 200 || apiErr.Code == nil || *apiErr.Code != 5001 {
		t.Fatalf("err = %#v, want *APIError 200 code 5001", err)
	}
	if got := s.hits.Load(); got != 1 || len(rec.got()) != 0 {
		t.Fatalf("requests = %d, sleeps = %d; want 1, 0", got, len(rec.got()))
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A dial error caused by the caller's own cancellation must surface as that cancellation
// and not as a ConnectionError, which would also be eligible for a retry. http.Transport
// itself prefers ctx.Err() over a racing dial error, so a RoundTripper pins the dial error.
func TestCancelDuringDialIsNotConnectionError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var dials atomic.Int32
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		dials.Add(1)
		cancel()
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	})
	tr, rec := testTransport("http://127.0.0.1:1", &http.Client{Transport: rt})
	tr.maxRetries = 0
	_, err := postBody(tr, ctx, []byte(`{}`), 5*time.Second, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	var ce *ConnectionError
	if errors.As(err, &ce) {
		t.Fatalf("caller cancellation reported as *ConnectionError: %v", err)
	}
	if got := dials.Load(); got != 1 || len(rec.got()) != 0 {
		t.Fatalf("dials = %d, sleeps = %d; want 1, 0", got, len(rec.got()))
	}
}
