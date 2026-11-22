package octen

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestZeroValuesAreSentWhenSet(t *testing.T) {
	got := mustMarshal(t, PlaceSearchParams{Query: "q", Latitude: Float64(0), Longitude: Float64(0), Radius: Int(0)})
	for _, want := range []string{`"latitude":0`, `"longitude":0`, `"radius":0`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s missing %s", got, want)
		}
	}
	if got := mustMarshal(t, PlaceSearchParams{Query: "q"}); got != `{"query":"q"}` {
		t.Errorf("unset optionals must be absent, got %s", got)
	}

	web := mustMarshal(t, SearchParams{Query: "q", Count: Int(0), IncludeImages: Bool(false), Format: String("")})
	for _, want := range []string{`"count":0`, `"include_images":false`, `"format":""`} {
		if !strings.Contains(web, want) {
			t.Errorf("%s missing %s", web, want)
		}
	}
	nested := mustMarshal(t, PulseSearchParams{Query: "q", Highlight: &HighlightOptions{Enable: Bool(false)}})
	if nested != `{"query":"q","highlight":{"enable":false}}` {
		t.Errorf("nested zero value: %s", nested)
	}
}

func TestEmptySlicesOmitted(t *testing.T) {
	got := mustMarshal(t, SearchParams{
		Query:          "q",
		IncludeDomains: []string{},
		ExcludeDomains: []string{},
		IncludeText:    []string{},
		ExcludeText:    []string{},
		Language:       []string{},
	})
	if got != `{"query":"q"}` {
		t.Errorf("empty slices must be omitted, got %s", got)
	}
	got = mustMarshal(t, SearchParams{Query: "q", Language: []string{"en"}})
	if got != `{"query":"q","language":["en"]}` {
		t.Errorf("non-empty slice: %s", got)
	}
}

func TestPtrHelpers(t *testing.T) {
	if *Int(3) != 3 || *String("a") != "a" || !*Bool(true) || *Float64(1.5) != 1.5 || *Ptr(uint8(7)) != 7 {
		t.Fatal("pointer helpers must return the value they were given")
	}
	a, b := Int(1), Int(1)
	if a == b {
		t.Fatal("each call must return a fresh pointer")
	}
}

type rawResponse interface{ RawJSON() []byte }

// setRaw stands in for the service layer, which fills the unexported raw field after decoding.
func setRaw(t *testing.T, v rawResponse, raw []byte) {
	t.Helper()
	switch r := v.(type) {
	case *SearchResponse:
		r.raw = raw
	case *BroadSearchResponse:
		r.raw = raw
	case *NewsSearchResponse:
		r.raw = raw
	case *BusinessSearchResponse:
		r.raw = raw
	case *PulseSearchResponse:
		r.raw = raw
	case *PlaceSearchResponse:
		r.raw = raw
	case *ImageSearchResponse:
		r.raw = raw
	case *VideoSearchResponse:
		r.raw = raw
	case *DesignSearchResponse:
		r.raw = raw
	default:
		t.Fatalf("unsupported response type %T", v)
	}
}

func TestUnknownResponseFieldsKeptInRawJSON(t *testing.T) {
	body := []byte(`{"code":0,"request_id":"r1","future":{"a":[1,2]},"data":{"query":"q","extra":1},"meta":{"latency":1.5}}`)
	cases := []struct {
		name string
		v    rawResponse
	}{
		{"search", &SearchResponse{}},
		{"broad", &BroadSearchResponse{}},
		{"news", &NewsSearchResponse{}},
		{"business", &BusinessSearchResponse{}},
		{"pulse", &PulseSearchResponse{}},
		{"place", &PlaceSearchResponse{}},
		{"image", &ImageSearchResponse{}},
		{"video", &VideoSearchResponse{}},
		{"design", &DesignSearchResponse{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.v.RawJSON(); got != nil {
				t.Fatalf("RawJSON before decode: %q", got)
			}
			if err := json.Unmarshal(body, c.v); err != nil {
				t.Fatalf("unknown fields must not fail decoding: %v", err)
			}
			setRaw(t, c.v, body)
			if !bytes.Equal(c.v.RawJSON(), body) {
				t.Errorf("RawJSON = %q", c.v.RawJSON())
			}
			if id := reflect.ValueOf(c.v).Elem().FieldByName("RequestID").String(); id != "r1" {
				t.Errorf("RequestID = %q", id)
			}
		})
	}
}

func TestSearchResponseDecodesAllFields(t *testing.T) {
	body := `{"code":0,"msg":"ok","request_id":"r","data":{"query":"q","results":[{
		"title":"T","url":"https://e.com","highlight":"h","full_content":"f","authors":"a",
		"time_published":"2025-01-01T00:00:00Z","time_last_crawled":"2025-01-02T00:00:00Z","favicon":"fav",
		"cover_image":{"url":"c","description":"cd"},"images":[{"url":"i","description":"id"}]}]},
		"meta":{"usage":{"num_search_queries":1,"full_content_extra_count":2},"result_type":"dynamic","latency":12.5,"warning":null}}`
	var r SearchResponse
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	if string(r.Code) != "0" || r.Msg == nil || *r.Msg != "ok" || r.RequestID != "r" {
		t.Errorf("envelope: %+v", r)
	}
	res := r.Data.Results[0]
	if res.Title != "T" || res.URL != "https://e.com" || res.CoverImage.Description != "cd" || res.Images[0].URL != "i" ||
		res.TimePublished != "2025-01-01T00:00:00Z" || res.TimeLastCrawled == "" {
		t.Errorf("result: %+v", res)
	}
	m := r.Meta
	if m.Usage.NumSearchQueries != 1 || m.Usage.FullContentExtraCount != 2 || m.ResultType != "dynamic" || m.Latency != 12.5 {
		t.Errorf("meta: %+v", m)
	}
	if m.Warning != nil {
		t.Errorf("null warning must decode to nil, got %q", *m.Warning)
	}
	var w SearchResponse
	if err := json.Unmarshal([]byte(`{"request_id":"r","meta":{"warning":"slow"}}`), &w); err != nil {
		t.Fatal(err)
	}
	if w.Meta.Warning == nil || *w.Meta.Warning != "slow" {
		t.Errorf("warning: %+v", w.Meta)
	}
}

func TestNullableResponseFieldsHoldNull(t *testing.T) {
	var p PulseSearchResponse
	body := `{"request_id":"r","data":{"results":[{"title":null,"url":"u"},{"title":"t","url":"v"}]},"meta":{"warning":null,"usage":{"billed_mode":"lite"}}}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	rs := p.Data.Results
	if rs[0].Title != nil || rs[1].Title == nil || *rs[1].Title != "t" {
		t.Errorf("titles: %+v", rs)
	}
	if p.Meta.Warning != nil || p.Meta.Usage.BilledMode != "lite" {
		t.Errorf("meta: %+v", p.Meta)
	}
	// Code may be anything on a success body; it is kept as raw JSON.
	for _, code := range []string{`null`, `"0"`, `true`, `0`} {
		var s SearchResponse
		if err := json.Unmarshal([]byte(`{"code":`+code+`,"request_id":"r"}`), &s); err != nil {
			t.Fatalf("code %s: %v", code, err)
		}
	}
}

func TestBroadSearchResponseDecodes(t *testing.T) {
	body := `{"code":0,"msg":"m","request_id":"r","data":{"query":"q","queries":["a","b"],
		"search_results":[{"query":"a","results":[{"url":"u"}],"latency":42}]},"meta":{"result_type":"standard"}}`
	var r BroadSearchResponse
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	g := r.Data.SearchResults[0]
	if r.Msg == nil || *r.Msg != "m" || len(r.Data.Queries) != 2 || g.Query != "a" || g.Latency != 42 || g.Results[0].URL != "u" {
		t.Errorf("broad: %+v", r)
	}
}

func TestNewsSearchResponseDecodes(t *testing.T) {
	body := `{"request_id":"r","data":{"query":"q","results":[{"url":"u","cover_image":{"url":"c"}}],
		"subjects":[{"name":"n","summary":"s","time_start":"a","time_latest":"b",
		"top_news":{"title":"t","summary":"ts","url":"tu","images":[{"url":"i"}]},
		"sub_news":[{"title":"st","url":"su"}]}]},
		"meta":{"usage":{"num_search_queries":1,"num_subject_search_queries":2,"full_content_extra_count":3}}}`
	var r NewsSearchResponse
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	s := r.Data.Subjects[0]
	if s.Name != "n" || s.TopNews.Summary != "ts" || s.TopNews.Images[0].URL != "i" || s.SubNews[0].URL != "su" ||
		r.Data.Results[0].CoverImage.URL != "c" || r.Meta.Usage.NumSubjectSearchQueries != 2 {
		t.Errorf("news: %+v", r)
	}
}

func TestPlaceSearchResponseDecodes(t *testing.T) {
	body := `{"request_id":"r","data":{"query":"q","location":{"latitude":0,"longitude":-1.5},
		"places":[{"name":"n","category":"c","address":"a","latitude":1.5,"longitude":2.5,"distance":120,"phone":"p",
		"website":"w","opening_hours":["Mon 9-5"],"status":"open","rating":4.5,"review_count":10,
		"attributes":{"wifi":true}}],"results":[{"url":"u"}]},"meta":{"latency":3}}`
	var r PlaceSearchResponse
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	p := r.Data.Places[0]
	if r.Data.Location.Longitude != -1.5 || p.Distance != 120 || p.ReviewCount != 10 || p.Status != "open" ||
		p.OpeningHours[0] != "Mon 9-5" || p.Attributes["wifi"] != true || r.Data.Results[0].URL != "u" {
		t.Errorf("place: %+v", r)
	}
}

func TestBusinessEntityUnknownType(t *testing.T) {
	in := `{"type":"fund","x":1}`
	var e BusinessEntity
	if err := json.Unmarshal([]byte(in), &e); err != nil {
		t.Fatal(err)
	}
	if e.Type != "fund" || e.Company != nil || e.Person != nil || string(e.Raw) != in {
		t.Errorf("unknown type: %+v", e)
	}
}

func TestBusinessEntityKnownTypes(t *testing.T) {
	company := `{"type":"company","name":"Acme","summary":"s","aliases":["A"],
		"identifiers":{"website":"w","linkedin_url":"l","stock_ticker":"ACM","sec_cik":"1"},
		"attributes":{"industry":"i","hq_location":"h","founded_year":1999,"employee_range":"1-10"},
		"metrics":{"financials":[{"revenue":1.5,"net_income":2,"gross_profit":3,"period":"FY","currency":"USD"}],
		"stock":{"price":9.5,"todays_change_percent":-1.5,"week_52_high":10,"week_52_low":5,"date":"d","currency":"USD"},
		"funding":[{"round":"A","amount":100,"date":"d","lead_investors":"x","currency":"USD"}],
		"web_traffic":{"visits_monthly":1000,"rank":7,"period":"p"}},
		"key_people":[{"name":"n","title":"CEO"}],"logo":{"url":"lu"},
		"activities":[{"url":"au","cover_image":{"url":"c"}}],"news":[{"url":"nu","images":[{"url":"i"}]}],"unknown":1}`
	var e BusinessEntity
	if err := json.Unmarshal([]byte(company), &e); err != nil {
		t.Fatal(err)
	}
	c := e.Company
	if e.Type != "company" || c == nil || e.Person != nil || string(e.Raw) != company {
		t.Fatalf("company entity: %+v", e)
	}
	if c.Name != "Acme" || c.Aliases[0] != "A" || c.Identifiers.SecCIK != "1" || c.Attributes.FoundedYear != 1999 ||
		c.Metrics.Financials[0].NetIncome != 2 || c.Metrics.Stock.Week52High != 10 || c.Metrics.Funding[0].LeadInvestors != "x" ||
		c.Metrics.WebTraffic.VisitsMonthly != 1000 || c.KeyPeople[0].Title != "CEO" || c.Logo.URL != "lu" ||
		c.Activities[0].CoverImage.URL != "c" || c.News[0].Images[0].URL != "i" {
		t.Errorf("company: %+v", c)
	}

	person := `{"type":"person","name":"Ann","summary":"s","linkedin_url":"l","current_position":"cto",
		"career":[{"organization":"o","title":"t","start":"2000","end":"2010"}],"photo":{"url":"pu"},
		"activities":[{"url":"au"}],"news":[{"url":"nu"}]}`
	var p BusinessEntity
	if err := json.Unmarshal([]byte(person), &p); err != nil {
		t.Fatal(err)
	}
	if p.Type != "person" || p.Person == nil || p.Company != nil || p.Person.CurrentPosition != "cto" ||
		p.Person.Career[0].End != "2010" || p.Person.Photo.URL != "pu" || p.Person.LinkedInURL != "l" {
		t.Errorf("person: %+v", p)
	}
}

func TestBusinessEntityEdgeCases(t *testing.T) {
	var noType BusinessEntity
	in := `{"name":"x"}`
	if err := json.Unmarshal([]byte(in), &noType); err != nil {
		t.Fatalf("entity without type must not fail: %v", err)
	}
	if noType.Type != "" || noType.Company != nil || noType.Person != nil || string(noType.Raw) != in {
		t.Errorf("no type: %+v", noType)
	}

	var badType BusinessEntity
	if err := json.Unmarshal([]byte(`{"type":5}`), &badType); err != nil || badType.Type != "" || string(badType.Raw) != `{"type":5}` {
		t.Errorf("non-string type: %+v err=%v", badType, err)
	}

	var bad BusinessEntity
	if err := json.Unmarshal([]byte(`{"type":"company","attributes":{"founded_year":"x"}}`), &bad); err == nil {
		t.Error("a known variant with a mistyped field must fail to decode")
	}

	var resp BusinessSearchResponse
	body := `{"request_id":"r","data":{"entities":[{"type":"company","name":"a"},{"type":"fund"},{"type":"person","name":"p"}]},"meta":{"warning":"w"}}`
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	es := resp.Data.Entities
	if len(es) != 3 || es[0].Company == nil || es[1].Type != "fund" || es[2].Person == nil || resp.Meta.Warning != "w" {
		t.Errorf("entities: %+v", es)
	}
}

func TestUndocumentedFieldsAbsent(t *testing.T) {
	banned := map[string]bool{
		"topic": true, "include_videos": true, "context": true,
		"num_highlights": true, "sentences_per_highlight": true, "html_snippet": true,
	}
	seen := map[reflect.Type]bool{}
	var walk func(path string, typ reflect.Type)
	walk = func(path string, typ reflect.Type) {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if banned[name] {
				t.Errorf("%s.%s exposes undocumented field %q", path, f.Name, name)
			}
			walk(path+"."+f.Name, f.Type)
		}
	}
	for name, v := range map[string]any{
		"SearchParams": SearchParams{}, "BroadSearchParams": BroadSearchParams{}, "NewsSearchParams": NewsSearchParams{},
		"BusinessSearchParams": BusinessSearchParams{}, "PulseSearchParams": PulseSearchParams{}, "PlaceSearchParams": PlaceSearchParams{},
		"ImageSearchParams": ImageSearchParams{}, "VideoSearchParams": VideoSearchParams{}, "DesignSearchParams": DesignSearchParams{},
	} {
		walk(name, reflect.TypeOf(v))
	}

	so, ok := reflect.TypeOf(BroadSearchParams{}).FieldByName("SearchOptions")
	if !ok {
		t.Fatal("BroadSearchParams.SearchOptions missing")
	}
	st := so.Type.Elem()
	for i := 0; i < st.NumField(); i++ {
		if name := strings.Split(st.Field(i).Tag.Get("json"), ",")[0]; name == "query" {
			t.Error("BroadSearchParams.SearchOptions must not carry query")
		}
	}
}

func TestRequiredQueryHasNoOmitempty(t *testing.T) {
	for _, v := range []any{SearchParams{}, BroadSearchParams{}, NewsSearchParams{}, BusinessSearchParams{}, PulseSearchParams{}, PlaceSearchParams{}} {
		f, ok := reflect.TypeOf(v).FieldByName("Query")
		if !ok || f.Type.Kind() != reflect.String || f.Tag.Get("json") != "query" {
			t.Errorf("%T.Query must be a plain string tagged json:\"query\"", v)
		}
	}
	if got := mustMarshal(t, SearchParams{}); got != `{"query":""}` {
		t.Errorf("empty query is still sent (validation is the caller's concern): %s", got)
	}
}

func TestEnumTagsOnRequestFields(t *testing.T) {
	check := func(typ reflect.Type, field, want string) {
		t.Helper()
		f, ok := typ.FieldByName(field)
		if !ok {
			t.Errorf("%s.%s missing", typ, field)
			return
		}
		if got := f.Tag.Get("enum"); got != want {
			t.Errorf("%s.%s enum = %q, want %q", typ, field, got, want)
		}
	}
	langs := "ar,de,en,es,fr,hi,id,it,ja,ko,nl,pl,pt,ru,th,tr,vi,zh"
	check(reflect.TypeOf(SearchParams{}), "ResultType", "standard,dynamic")
	check(reflect.TypeOf(SearchParams{}), "TimeBasis", "auto,published,crawled")
	check(reflect.TypeOf(SearchParams{}), "TimeRange", "day,week,month,year,d,w,m,y")
	check(reflect.TypeOf(SearchParams{}), "Language", langs)
	check(reflect.TypeOf(SearchParams{}), "Format", "markdown,text")
	check(reflect.TypeOf(SearchParams{}), "Safesearch", "off,strict")
	check(reflect.TypeOf(WebSearchOptions{}), "ResultType", "standard,dynamic")
	check(reflect.TypeOf(PulseSearchParams{}), "Mode", "lite,standard")
	check(reflect.TypeOf(PulseSearchParams{}), "Safesearch", "off,strict")
	check(reflect.TypeOf(PlaceUserLocation{}), "Type", "approximate")
	check(reflect.TypeOf(PlaceWebSearchOptions{}), "Language", langs)
	check(reflect.TypeOf(ImageSearchInput{}), "Type", "text,image")
	check(reflect.TypeOf(ImageSearchParams{}), "Safesearch", "off,strict")
	check(reflect.TypeOf(VideoSearchInput{}), "Type", "text")
	check(reflect.TypeOf(VideoSearchParams{}), "TimeRange", "day,week,month,year,d,w,m,y")
	check(reflect.TypeOf(VideoSearchParams{}), "Safesearch", "off,strict")
	check(reflect.TypeOf(DesignWebSearchOptions{}), "Language", langs)
	check(reflect.TypeOf(DesignWebSearchOptions{}), "TimeBasis", "auto,published,crawled")
	check(reflect.TypeOf(DesignWebSearchOptions{}), "TimeRange", "day,week,month,year,d,w,m,y")
	check(reflect.TypeOf(ExtractParams{}), "Mode", "standard,advanced,auto")
	check(reflect.TypeOf(ExtractParams{}), "Format", "markdown,text")
	check(reflect.TypeOf(ExtractLinksOptions{}), "Scope", "prefer_internal,prefer_external")
}

// A decoded response must survive json.Marshal and json.Unmarshal, so callers can cache it.
func TestBusinessEntityMarshalRoundTrip(t *testing.T) {
	body := `{"request_id":"r","data":{"entities":[
		{"type":"company","name":"Acme","identifiers":{"stock_ticker":"ACM"}},
		{"type":"person","name":"Ann","current_position":"cto"},
		{"type":"fund","x":1}]}}`
	var in BusinessSearchResponse
	if err := json.Unmarshal([]byte(body), &in); err != nil {
		t.Fatal(err)
	}
	var out BusinessSearchResponse
	if err := json.Unmarshal([]byte(mustMarshal(t, &in)), &out); err != nil {
		t.Fatal(err)
	}
	es := out.Data.Entities
	if len(es) != 3 {
		t.Fatalf("entities after round trip: %+v", es)
	}
	if es[0].Company == nil || !reflect.DeepEqual(es[0].Company, in.Data.Entities[0].Company) || es[0].Company.Name != "Acme" {
		t.Errorf("company after round trip: %+v", es[0].Company)
	}
	if es[1].Person == nil || !reflect.DeepEqual(es[1].Person, in.Data.Entities[1].Person) || es[1].Person.CurrentPosition != "cto" {
		t.Errorf("person after round trip: %+v", es[1].Person)
	}
	if es[2].Type != "fund" || string(es[2].Raw) != `{"type":"fund","x":1}` {
		t.Errorf("unknown type after round trip: %+v (raw %s)", es[2], es[2].Raw)
	}

	// Without Raw, the variant is marshalled; with nothing at all, the entity is null.
	built := BusinessEntity{Person: &PersonEntity{Type: "person", Name: "Bo"}}
	if got := mustMarshal(t, built); got != `{"type":"person","name":"Bo"}` {
		t.Errorf("built person = %s", got)
	}
	if got := mustMarshal(t, BusinessEntity{Company: &CompanyEntity{Type: "company", Name: "C"}}); got != `{"type":"company","name":"C"}` {
		t.Errorf("built company = %s", got)
	}
	if got := mustMarshal(t, BusinessEntity{}); got != "null" {
		t.Errorf("empty entity = %s, want null", got)
	}
}

// Bodies below follow live responses of 2026-11-21.
func TestImageSearchResponseDecodes(t *testing.T) {
	body := `{"data":{"results":[{"title":"T","url":"https://e.com/a.jpg","source_page":"https://e.com/a",
		"description":"d","width":1024,"height":683,"thumbnail":"https://cdn.example/t.jpg"}]},
		"request_id":"r","meta":{"usage":{"num_search_queries":1},"latency":419,"warning":""},"code":0,"msg":""}`
	var r ImageSearchResponse
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	want := ImageSearchResult{Title: "T", URL: "https://e.com/a.jpg", SourcePage: "https://e.com/a", Description: "d",
		Width: 1024, Height: 683, Thumbnail: "https://cdn.example/t.jpg"}
	if len(r.Data.Results) != 1 || r.Data.Results[0] != want {
		t.Errorf("results: %+v", r.Data.Results)
	}
	if r.RequestID != "r" || r.Meta.Usage.NumSearchQueries != 1 || r.Meta.Latency != 419 || r.Meta.Warning == nil || *r.Meta.Warning != "" {
		t.Errorf("envelope: %+v meta %+v", r, r.Meta)
	}
	var sparse ImageSearchResponse
	if err := json.Unmarshal([]byte(`{"request_id":"r","data":{"results":[{"url":"u"}]},"meta":{"warning":null,"usage":null}}`), &sparse); err != nil {
		t.Fatal(err)
	}
	if sparse.Data.Results[0].Title != "" || sparse.Meta.Warning != nil || sparse.Meta.Usage != nil || sparse.Code != nil || sparse.Msg != nil {
		t.Errorf("sparse: %+v meta %+v", sparse, sparse.Meta)
	}
	var empty ImageSearchResponse
	if err := json.Unmarshal([]byte(`{"request_id":"r"}`), &empty); err != nil || empty.Data != nil || empty.Meta != nil {
		t.Errorf("missing data and meta: %+v err=%v", empty, err)
	}
}

func TestVideoSearchResponseDecodes(t *testing.T) {
	body := `{"data":{"results":[{"title":"T","url":"https://v.example/w","source_page":"https://v.example/p",
		"description":"d","cover_url":"https://v.example/c.jpg","duration_seconds":78,"authors":"A",
		"time_published":"2011-04-16T00:00:00Z","time_last_crawled":"2026-05-28T05:04:41Z"}]},
		"request_id":"r","meta":{"usage":{"num_search_queries":1},"latency":46,"warning":""},"code":0,"msg":""}`
	var r VideoSearchResponse
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	want := VideoSearchResult{Title: "T", URL: "https://v.example/w", SourcePage: "https://v.example/p", Description: "d",
		CoverURL: "https://v.example/c.jpg", DurationSeconds: 78, Authors: "A",
		TimePublished: "2011-04-16T00:00:00Z", TimeLastCrawled: "2026-05-28T05:04:41Z"}
	if len(r.Data.Results) != 1 || r.Data.Results[0] != want {
		t.Errorf("results: %+v", r.Data.Results)
	}
	if r.Meta.Usage.NumSearchQueries != 1 || r.Meta.Latency != 46 {
		t.Errorf("meta: %+v", r.Meta)
	}
	var sparse VideoSearchResponse
	if err := json.Unmarshal([]byte(`{"request_id":"r","data":{"results":null},"meta":{"warning":null}}`), &sparse); err != nil {
		t.Fatal(err)
	}
	if sparse.Data.Results != nil || sparse.Meta.Warning != nil {
		t.Errorf("sparse: %+v", sparse)
	}
}

func TestDesignSearchResponseDecodes(t *testing.T) {
	body := `{"data":{"query":"q","results":[{"title":"T","url":"https://w.example","highlight":"h","full_content":"f",
		"authors":"A","time_published":"2026-07-04T13:26:15Z","time_last_crawled":"2026-11-21T11:32:33Z","favicon":"fav",
		"cover_image":{"url":"c","description":""},"images":[{"url":"i","description":"id"}]}],
		"images":[{"type":"web_section","title":"Hero","description":"desc","url":"https://cdn.example/s.png",
		"thumbnail":"https://cdn.example/t.png","source_page":"https://site.example/","width":1440,"height":900,
		"group":{"group_id":"pg_1","type":"web_page","name":"Site","position":1,"total":18},
		"design_facts":{"color":{"text":{"$type":"color"}},"fonts":["Switzer"]},
		"snippet":{"format":"html","code":"<section></section>"}},
		{"type":"slide","url":"u2","group":{"group_id":"deck_1","type":"deck","position":19,"total":28,"master":{"k":"v"}}},
		{"type":"app_screen","url":"u3","group":null,"design_facts":null,"snippet":null}],
		"specs":[{"undocumented":true}]},
		"request_id":"r","meta":{"usage":{"num_search_queries":1},"latency":263},"code":0,"msg":"success"}`
	var r DesignSearchResponse
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	d := r.Data
	if d.Query != "q" || len(d.Images) != 3 || len(d.Results) != 1 {
		t.Fatalf("data: %+v", d)
	}
	img := d.Images[0]
	if img.Type != "web_section" || img.Title != "Hero" || img.Description != "desc" || img.URL != "https://cdn.example/s.png" ||
		img.Thumbnail != "https://cdn.example/t.png" || img.SourcePage != "https://site.example/" || img.Width != 1440 || img.Height != 900 {
		t.Errorf("image: %+v", img)
	}
	if g := img.Group; g == nil || g.GroupID != "pg_1" || g.Type != "web_page" || g.Name != "Site" || g.Position != 1 || g.Total != 18 || g.Master != nil {
		t.Errorf("group: %+v", img.Group)
	}
	if img.Snippet == nil || img.Snippet.Format != "html" || img.Snippet.Code != "<section></section>" {
		t.Errorf("snippet: %+v", img.Snippet)
	}
	if fonts, ok := img.DesignFacts["fonts"].([]any); !ok || fonts[0] != "Switzer" || img.DesignFacts["color"] == nil {
		t.Errorf("design_facts: %+v", img.DesignFacts)
	}
	if g := d.Images[1].Group; g == nil || g.Type != "deck" || g.Position != 19 || g.Master["k"] != "v" {
		t.Errorf("deck group: %+v", g)
	}
	if bare := d.Images[2]; bare.Group != nil || bare.DesignFacts != nil || bare.Snippet != nil {
		t.Errorf("null group, design_facts and snippet must decode to nil: %+v", bare)
	}
	res := d.Results[0]
	if res.Title != "T" || res.URL != "https://w.example" || res.Highlight != "h" || res.FullContent != "f" || res.Authors != "A" ||
		res.TimePublished == "" || res.TimeLastCrawled == "" || res.Favicon != "fav" || res.CoverImage == nil || res.CoverImage.URL != "c" ||
		len(res.Images) != 1 || res.Images[0].Description != "id" {
		t.Errorf("web result: %+v", res)
	}
	if r.Msg == nil || *r.Msg != "success" || r.Meta.Usage.NumSearchQueries != 1 || r.Meta.Latency != 263 || r.Meta.Warning != nil {
		t.Errorf("envelope: %+v meta %+v", r, r.Meta)
	}
	// An images.enable or web_search.enable of false leaves the list out or empty.
	var off DesignSearchResponse
	if err := json.Unmarshal([]byte(`{"request_id":"r","data":{"query":"q","images":[],"results":null}}`), &off); err != nil {
		t.Fatal(err)
	}
	if len(off.Data.Images) != 0 || off.Data.Results != nil || off.Meta != nil {
		t.Errorf("disabled lists: %+v", off.Data)
	}
}
