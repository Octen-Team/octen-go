# octen-go

Official Go SDK for the [Octen](https://docs.octen.ai) search API. It has no dependencies outside the standard library and needs Go 1.22 or newer.

It covers ten endpoints: Web Search, Web Search Premier, Broad Search, News Search, Business Search, Pulse Search, Place Search, Image Search, Video Search (invite-only beta) and Design Search.

- [Install](#install)
- [Quick start](#quick-start)
- [Methods](#methods)
- [Request options](#request-options)
- [Client options](#client-options)
- [Errors](#errors)
- [Retries and timeouts](#retries-and-timeouts)
- [Differences from the Python SDK](#differences-from-the-python-sdk)
- [Differences from the TypeScript SDK](#differences-from-the-typescript-sdk)
- [Versioning and releasing](#versioning-and-releasing)
- [Development](#development)

## Install

```bash
go get github.com/Octen-Team/octen-go@v0.1.0
```

Create an API key in your Octen account and either pass it with `octen.WithAPIKey` or set the `OCTEN_API_KEY` environment variable. Surrounding whitespace is trimmed from the key.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	octen "github.com/Octen-Team/octen-go"
)

func main() {
	client, err := octen.NewClient() // reads OCTEN_API_KEY; or: octen.NewClient(octen.WithAPIKey("sk-..."))
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Search.Search(context.Background(), octen.SearchParams{
		Query: "latest advances in solid-state batteries",
		Count: octen.Int(5),
	})
	if err != nil {
		log.Fatal(err)
	}
	if resp.Data != nil {
		for _, r := range resp.Data.Results {
			fmt.Println(r.Title, r.URL)
		}
	}
}
```

Always create a client with `NewClient`; the zero value of `Client` is not usable. A client is safe for concurrent use, so share one for the life of the program.

Field names follow the [API reference](https://docs.octen.ai) exactly, in Go style: `include_domains` is `IncludeDomains`, `request_id` is `RequestID`, `url` is `URL`. Required fields are plain values: `Query`, and for Image and Video Search the `Inputs` slice and each input's `Type`. Every other request scalar is a pointer (including the optional `Query` of Design Search), so "not set" and "zero" stay distinct (a latitude of `0` is a real value); the helpers `octen.Int`, `octen.String`, `octen.Bool`, `octen.Float64` and the generic `octen.Ptr` build those pointers. Nested option objects are pointers to structs, and lists are slices.

Responses are the whole JSON envelope: `Code`, `Msg`, `RequestID`, `Data` and `Meta`. `RequestID` is the body's `request_id`, or an empty string when the body has none; `Data` and `Meta` are pointers that are `nil` when the server did not send them. `resp.RawJSON()` returns the body exactly as received, including fields this version does not model.

## Methods

Every method takes a `context.Context`, the request struct, and optional [request options](#request-options). `Query` is required, except that Image and Video Search take exactly one entry in `Inputs` instead, and Design Search may leave it out when `Images.GroupID` is set.

The snippets below check `err` and `Data != nil` inline to stay short; `Data` and `Meta` are pointers that are `nil` when the server did not send them.

```go
ctx := context.Background()

// Web Search: POST /search
web, err := client.Search.Search(ctx, octen.SearchParams{
	Query:          "rust async runtimes",
	Count:          octen.Int(10),
	IncludeDomains: []string{"github.com"},
	TimeRange:      octen.String("month"),
	Highlight:      &octen.HighlightOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(300)},
})
if err == nil && web.Data != nil && len(web.Data.Results) > 0 {
	fmt.Println(web.Data.Results[0].Highlight)
}

// Broad Search: POST /broad-search (rewrites the question and runs several sub-queries)
broad, err := client.Search.BroadSearch(ctx, octen.BroadSearchParams{
	Query:      "compare central bank rate decisions this quarter",
	MaxQueries: octen.Int(5),
	SearchOptions: &octen.WebSearchOptions{
		Count:    octen.Int(5),
		Language: []string{"en"},
	},
})
if err == nil && broad.Data != nil {
	fmt.Println(broad.Data.Queries)
	for _, group := range broad.Data.SearchResults {
		fmt.Println(group.Query, len(group.Results))
	}
}

// News Search: POST /news-search
news, err := client.Search.NewsSearch(ctx, octen.NewsSearchParams{
	Query:     "Federal Reserve interest rate decision",
	Count:     octen.Int(5),
	TimeRange: octen.String("week"),
	Subjects:  &octen.NewsSubjectOptions{Enable: octen.Bool(true), Count: octen.Int(3), MaxSubNews: octen.Int(5)},
})
if err == nil && news.Data != nil {
	for _, s := range news.Data.Subjects {
		fmt.Println(s.Name)
	}
}

// Business Search: POST /business-search (company and person entity cards)
biz, err := client.Search.BusinessSearch(ctx, octen.BusinessSearchParams{
	Query: "Example Semiconductor quarterly results",
	Count: octen.Int(5),
	Entities: &octen.BusinessEntityOptions{
		Enable:        octen.Bool(true),
		Count:         octen.Int(2),
		MaxActivities: octen.Int(5),
		MaxNews:       octen.Int(5),
	},
})
if err == nil && biz.Data != nil {
	for _, e := range biz.Data.Entities {
		switch {
		case e.Company != nil:
			if ids := e.Company.Identifiers; ids != nil {
				fmt.Println("company", e.Company.Name, ids.StockTicker)
			}
		case e.Person != nil:
			fmt.Println("person", e.Person.Name, e.Person.CurrentPosition)
		default:
			// The server may add new entity types; Type and Raw are still filled.
			fmt.Println("other entity type", e.Type, string(e.Raw))
		}
	}
}

// Pulse Search: POST /pulse-search (mode "lite" returns URL and title only)
pulse, err := client.Search.PulseSearch(ctx, octen.PulseSearchParams{
	Query:       "python list methods",
	Mode:        octen.String("standard"),
	Count:       octen.Int(3),
	FullContent: &octen.FullContentOptions{Enable: octen.Bool(true), MaxTokens: octen.Int(1000)},
})
if err == nil && pulse.Meta != nil && pulse.Meta.Usage != nil {
	fmt.Println(pulse.Meta.Usage.BilledMode)
}

// Place Search: POST /place-search (note: client.PlaceSearch, not client.Search)
places, err := client.PlaceSearch.Search(ctx, octen.PlaceSearchParams{
	Query:     "Korean restaurants",
	Places:    &octen.PlaceCardOptions{Count: octen.Int(5)},
	Latitude:  octen.Float64(37.788),
	Longitude: octen.Float64(-122.4075),
	Radius:    octen.Int(2000), // meters, needs latitude and longitude
})
if err == nil && places.Data != nil {
	for _, p := range places.Data.Places {
		fmt.Println(p.Name, p.Distance, p.Rating)
	}
}

// Web Search Premier: POST /search-premier (deeper retrieval and reranking; same params and response as Search)
premier, err := client.Search.SearchPremier(ctx, octen.SearchParams{
	Query:          "EU AI Act guidance",
	ResultType:     octen.String("dynamic"), // only relevant results, up to Count
	Count:          octen.Int(5),
	IncludeDomains: []string{"europa.eu"},
})
if err == nil && premier.Meta != nil && premier.Data != nil {
	fmt.Println(premier.Meta.ResultType, len(premier.Data.Results))
}

// Image Search: POST /image-search (exactly one input: a text query, or an image as a URL or base64 data)
images, err := client.Search.ImageSearch(ctx, octen.ImageSearchParams{
	Inputs: []octen.ImageSearchInput{{Type: "image", URL: octen.String("https://example.com/photo.jpg")}},
	// or an uploaded image: {Type: "image", Data: octen.String(base64.StdEncoding.EncodeToString(jpegBytes))}
	// or a text query:      {Type: "text", Data: octen.String("golden retriever puppy")}
	Count: octen.Int(5),
})
if err == nil && images.Data != nil {
	for _, r := range images.Data.Results {
		fmt.Println(r.Title, r.URL, r.Width, r.Height)
	}
}

// Video Search: POST /video-search (invite-only beta: contact Octen for access)
videos, err := client.Search.VideoSearch(ctx, octen.VideoSearchParams{
	Inputs:    []octen.VideoSearchInput{{Type: "text", Data: octen.String("how to tie a bowline knot")}},
	Count:     octen.Int(3),
	TimeRange: octen.String("year"),
})
if err == nil && videos.Data != nil {
	for _, r := range videos.Data.Results {
		fmt.Println(r.Title, r.URL, r.DurationSeconds)
	}
}

// Design Search: POST /design-search (design references as image results, plus web results)
design, err := client.Search.DesignSearch(ctx, octen.DesignSearchParams{
	Query:     octen.String("SaaS landing page hero section"),
	Images:    &octen.DesignImageOptions{Count: octen.Int(5), Snippet: &octen.DesignSnippetOptions{MaxTokens: octen.Int(2000)}},
	WebSearch: &octen.DesignWebSearchOptions{Count: octen.Int(3)},
})
if err == nil && design.Data != nil {
	for _, img := range design.Data.Images {
		fmt.Println(img.Type, img.URL, img.DesignFacts)
		if img.Group != nil {
			// The rest of the page or deck, in order (Position runs from 1 to Total):
			// client.Search.DesignSearch(ctx, octen.DesignSearchParams{
			//	Images: &octen.DesignImageOptions{GroupID: octen.String(img.Group.GroupID)}})
			fmt.Println(img.Group.Name, img.Group.Position, img.Group.Total)
		}
	}
}
```

Web Search Premier takes `SearchParams` and returns `*SearchResponse`, the same types as Web Search, so code written for one works with the other. Video Search is an invite-only beta; with a key that has no access the server refuses the call. In Design Search, `DesignFacts` and `Group.Master` are free-form maps decided by the server; set `Images.DesignFacts` or `Images.Snippet.Enable` to `false` to leave them out, and `Images.Enable` or `WebSearch.Enable` to `false` to skip a result list.

`BusinessEntity` is a union of `Company` and `Person`, selected by the entity's `type`. For a `type` this version does not know, or a missing one, only `Type` and `Raw` are filled, so a new entity type never breaks decoding: always keep a `default` branch. `Raw` holds the entity's original JSON, and `json.Marshal` writes it back unchanged, so a decoded response can be stored and decoded again. An entity built by hand without `Raw` marshals as its `Company` or `Person`.

### Local validation

Before sending anything the SDK checks that `Query` is not blank (surrounding whitespace does not count) and rejects `NaN` and `±Inf` anywhere in the params, because `encoding/json` cannot encode them; both are reported as a `*ValidationError` with the offending field. Image and Video Search instead need exactly one input in `Inputs` (`Field` is `inputs`) whose `Type` is `text` or `image` for Image Search and `text` for Video Search (`inputs[0].type`); a text input needs non-blank `Data` (`inputs[0].data`) and no `URL` (`inputs[0].url`), and an image input needs exactly one of a non-blank `URL` or base64 `Data` (`inputs[0]`); Design Search needs a non-blank `Query` or a non-blank `Images.GroupID` (`Field` is `query`). Apart from the input `Type`, it does not check value ranges or enum values (for example `Count` or `TimeRange`); out-of-range values come back from the server as a 400 and match `ErrBadRequest`.

Slices are omitted from the request when they are empty, so an empty slice cannot be sent explicitly.

## Request options

Each method takes optional `octen.RequestOption` values after the params:

| Option | Meaning |
|---|---|
| `octen.WithRequestTimeout(d)` | Timeout of each attempt of this call. Precedence: this option, then an explicit client `WithTimeout`, then the endpoint default (120 s for Broad Search, 30 s otherwise) |
| `octen.WithRequestHeader(name, value)` | Extra header for this call, replacing a client header of the same name. `x-api-key` is refused with a `*ValidationError`; the key only comes from `WithAPIKey` or the `OCTEN_API_KEY` environment variable |

There is no per-call cancellation option: pass a `context.Context` and cancel it.

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
resp, err := client.Search.Search(ctx, octen.SearchParams{Query: "hello"},
	octen.WithRequestTimeout(10*time.Second))
```

## Client options

```go
client, err := octen.NewClient(
	octen.WithAPIKey("sk-..."),                // default: $OCTEN_API_KEY; NewClient fails if neither is set
	octen.WithBaseURL("https://api.octen.ai"), // default shown
	octen.WithTimeout(60*time.Second),         // per attempt; applies to every endpoint once set
	octen.WithMaxRetries(3),                   // default 3 (at most 4 requests)
	octen.WithHeader("X-Trace-Id", "abc"),     // sent with every request; must not be x-api-key
	octen.WithHTTPClient(myHTTPClient),        // default: a fresh http.Client
)
```

- The timeout counts each attempt and includes reading the response body. Once `WithTimeout` is set it applies to every endpoint, Broad Search included; without it, Broad Search defaults to 120 s (it runs many sub-queries) and every other endpoint to 30 s. `WithRequestTimeout` beats both.
- Only the `x-api-key` header carries credentials. Redirects are never followed (a redirect is reported as an `*APIError`), so the key cannot be forwarded to another host.
- The `User-Agent` is `octen-go-sdk/<version>`.
- `WithHeader` and `WithRequestHeader` can override `Content-Type`, `Accept` and `User-Agent`. Don't: the API expects JSON in both directions.
- `WithHTTPClient` does not modify the client you pass in: the SDK works on a copy and sets its own `CheckRedirect` on the copy. Leave `http.Client.Timeout` at 0 there; see [Retries and timeouts](#retries-and-timeouts).
- Invalid options (no key, a malformed `WithBaseURL` or one with `user:password@`, a query, a fragment or a port outside 1-65535, a timeout that is not greater than 0, a negative retry count, an invalid header name, a header value or API key with a control character other than tab, a nil `http.Client`) make `NewClient` return a `*ValidationError` whose `Field` names the option. A bad per-call option is returned by the method call the same way.

## Errors

The SDK defines four error types and five sentinels:

| Situation | Error |
|---|---|
| 401 | `*APIError`, `errors.Is(err, octen.ErrAuthentication)` |
| 403 (no permission, insufficient balance) | `*APIError`, `octen.ErrPermissionDenied` |
| 400, 422 | `*APIError`, `octen.ErrBadRequest` |
| 429 (retries used up, or `Retry-After` above 60 s) | `*APIError`, `octen.ErrRateLimit`; `RetryAfter` is set when the header was valid |
| 5xx (500, 502, 503, 504 once retries are used up, or a 503 whose `Retry-After` is above 60 s; any other 5xx at once) | `*APIError`, `octen.ErrServer` |
| Any other non-2xx; a 2xx that is not a usable result (see below) | `*APIError`, matches no sentinel |
| An attempt exceeded its timeout | `*TimeoutError` |
| Network error | `*ConnectionError` (the cause is in `Err`) |
| Rejected by the SDK before sending | `*ValidationError` (has `Field`) |
| Your own context was cancelled or hit its deadline | an error wrapping `context.Canceled` or `context.DeadlineExceeded` |

A 2xx response counts as a failure when the body is not a non-empty JSON object (HTML, an array, `{}`, an empty body) or when it carries a numeric `code` other than 0. A 2xx whose body is a successful envelope but cannot be decoded into the typed response (the server sent a type the OpenAPI spec does not describe) is also an `*APIError`, with a `Message` starting `decode response:` and the raw body in `Body`.

```go
resp, err := client.Search.Search(ctx, octen.SearchParams{Query: "hello"})
if err != nil {
	var apiErr *octen.APIError
	var timeoutErr *octen.TimeoutError
	var connErr *octen.ConnectionError
	switch {
	case errors.As(err, &timeoutErr):
		// Check this first: an SDK timeout also satisfies errors.Is(err, context.DeadlineExceeded).
		log.Println("attempt timed out after", timeoutErr.Timeout)
	case errors.As(err, &connErr):
		// Before the context check: on Go 1.23 and newer an http.Client.Timeout also satisfies context.DeadlineExceeded.
		log.Println("network error:", connErr.Err)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		// Your own context ended.
		log.Println("stopped:", err)
	case errors.Is(err, octen.ErrRateLimit):
		if errors.As(err, &apiErr) && apiErr.RetryAfter != nil {
			log.Println("slow down, retry after", *apiErr.RetryAfter)
		}
	case errors.As(err, &apiErr):
		// StatusCode, Code (*float64), Message, RequestID, Body (raw bytes).
		log.Println(apiErr.StatusCode, apiErr.Message, apiErr.RequestID)
	default:
		log.Fatal(err) // *ValidationError
	}
	return
}
_ = resp
```

Check for `*TimeoutError` with `errors.As` before anything else. `errors.Is(err, context.DeadlineExceeded)` is also true for it, and, on Go 1.23 and newer, for a `*ConnectionError` caused by a non-zero `http.Client.Timeout`; only the `*TimeoutError` and `*ConnectionError` targets tell those apart from the deadline of a context you passed in.

`APIError.Message` is taken from the response body's `msg`, `message`, `detail` or `error` field (the first non-empty string, in that order; when `error` is an object, the same keys are searched inside it), else the first 500 UTF-16 code units of the body (characters for most text), else `HTTP <status>`. A redirect (3xx) instead gets a message saying that redirects are not followed and to check the base URL. `RequestID` comes from the `x-request-id` header, else the body's `request_id`, and is empty when the response has neither; quote it when contacting support. `Error()` reads `octen: <status> <message>` for API errors.

## Retries and timeouts

**Retried automatically** (up to `WithMaxRetries` times, default 3):

- HTTP 408, 429, 500, 502, 503 and 504. Every other status, including the other 5xx such as 501 and 505, fails at once.
- Network errors that happen before the request is sent: a failure while dialling (a `*net.OpError` whose `Op` is `dial`) or a DNS error (`*net.DNSError`).

**Waiting between attempts:** `1 s * 2^(n-1)` before retry `n`, plus up to 25% random jitter, at most 60 s. A `Retry-After` header (whole seconds or an HTTP date) is honoured on 429 and 503 only; if it asks for more than 60 s the SDK stops and returns the error at once. `APIError.RetryAfter` is set for 429 responses only, rounded up to whole seconds.

**Not retried:**

- An SDK timeout (`*TimeoutError`). The server may have finished and billed the request by then, so a blind retry could bill twice. The caller decides whether to resend.
- Cancellation or a deadline of your own context: the SDK stops immediately.
- Network errors after the request was sent (a connection reset mid-response, an edge gateway recycling the connection) and any network error that is not a dial or DNS failure.
- Other 4xx statuses.

**Timeouts.** Each attempt gets its own timeout, which covers connecting, sending, and reading the whole response body. Precedence: `WithRequestTimeout`, then an explicit `WithTimeout`, then the endpoint default (30 s; 120 s for Broad Search).

**`http.Client.Timeout`.** If you pass a client with `WithHTTPClient`, leave its `Timeout` at 0 and use the SDK timeouts instead. A non-zero `Timeout` also applies on top of them; when it fires it surfaces as a `*ConnectionError`, not a `*TimeoutError`, it is never retried (not even while dialling), and on Go 1.23 and newer it also satisfies `errors.Is(err, context.DeadlineExceeded)`.

**Idempotency-Key.** Do not add an `Idempotency-Key` or `X-Idempotency-Key` header with `WithHeader` or `WithRequestHeader`. Go's `net/http` treats a POST that carries either one as replayable and may resend it on its own when a reused connection turns out to be broken, outside the SDK's retry rules. The API does not support idempotency keys, so such a replay can bill the request twice.

**Known residual risk of double billing.** The API has no idempotency keys yet, so two situations can bill a request twice or bill a request that looked like it failed:

1. An edge gateway or load balancer returns 408, 502, 503 or 504 after the backend already succeeded; the retry then runs and is billed again.
2. The server records the billing event but fails while writing the response (for example the connection is recycled mid-response). You see a network error, the SDK does not retry it, but the call was billed.

For billing-sensitive jobs, use `octen.WithMaxRetries(0)` and handle retries yourself. Server-side idempotency keys are the proper fix and are not available yet.

## Differences from the Python SDK

Compared with `octen-py` 0.11.1:

| Area | `octen-py` | `octen-go` |
|---|---|---|
| Parameter validation | Checks value ranges (such as `count`) locally | Checks only the required inputs (a non-blank `Query`, one Image or Video Search input) and that numbers are finite; ranges are checked by the server (400) |
| Image Search input | `query=` or `image_url=` keyword (or a raw `inputs=` list) | One `ImageSearchInput` in `Inputs` |
| Unknown keys in nested request objects | Top level rejected, nested `highlight` / `full_content` allowed | Not possible: requests are typed structs |
| Client timeout | Retried | Not retried |
| `Retry-After` | 429 only | 429 and 503; above 60 s no retry |
| Network errors | All retried | Only errors before the request is sent (dial and DNS) |
| 2xx responses | Validated against the full response model | Judged only by the rules under [Errors](#errors), plus decoding into the typed response |
| Error classes | Dedicated classes for 401 and 429 | Also 403, 400/422 and 5xx sentinels |
| Auth headers | `x-api-key` and `Authorization: Bearer` | `x-api-key` only |
| Deprecated `topic` parameter, and `html_snippet` on Image Search | Still present | Not exposed |
| Broad Search timeout | 30 s like every endpoint | 120 s, separate default, unless a client timeout is set explicitly |

Because the Python SDK retries more situations (client timeouts and every network error), it carries the double-billing risks described above and more.

## Differences from the TypeScript SDK

Behaviour is aligned with `@octen.ai/sdk`. The differences are:

- A 2xx whose body is a successful envelope but does not decode into the typed response returns an `*APIError` (`decode response: ...`) that matches no sentinel. The call was billed, and the data is in `Body`, the raw response bytes; do not retry it. The TypeScript SDK does not check response structure and returns such a body untyped.
- The API key is trimmed of surrounding whitespace, and an empty or blank `WithAPIKey` falls back to `OCTEN_API_KEY`. The TypeScript SDK rejects an explicit empty key.
- Every network error while dialling is retried (a refused connection, a dial timeout of the transport's dialer, an unreachable network or host), and so is every DNS error. The TypeScript SDK retries only `ECONNREFUSED`, `ENOTFOUND` and `EAI_AGAIN`.
- A `Retry-After` HTTP date is parsed strictly with `http.ParseTime` (RFC 1123, RFC 850 or ANSI C format); anything else falls back to the normal backoff. The TypeScript SDK passes any value shaped like an HTTP date to `Date.parse`, which is more lenient.

Everything else that differs follows from Go: methods take a `context.Context` instead of an `AbortSignal`, errors are values matched with `errors.Is` and `errors.As`, and request fields are typed structs.

## Versioning and releasing

The SDK follows semantic versioning. Before 1.0, a minor release may contain incompatible changes; they are listed in [CHANGELOG.md](CHANGELOG.md). Published versions are permanent in the Go module proxy and cannot be deleted; a bad release is withdrawn by adding a `retract` directive to `go.mod` in a later release.

To release, tag `vX.Y.Z` on `main` after CI is green; the release workflow reruns the CI checks (including staticcheck on the latest Go) on Go 1.22 and the latest Go, checks the tag against `octen.Version` and that the commit is on `main`, then warms the module proxy.

## Development

```bash
go vet ./...
go test -race -count=1 ./...
OCTEN_LIVE=1 OCTEN_API_KEY=... go test -count=1 -run TestLive ./...   # real, billed calls
go run ./internal/tools/specdrift   # compares the live OpenAPI spec with testdata/openapi.json
```

`testdata/openapi.json` is a snapshot of the public OpenAPI spec that the contract test compares the types against. A weekly workflow runs `specdrift` and fails when the schemas of the ten endpoints change upstream; it never rewrites the snapshot. [testdata/README.md](testdata/README.md) describes the snapshot and how to update it.

## License

MIT
