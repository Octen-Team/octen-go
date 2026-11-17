package octen

import "encoding/json"

// BusinessEntityOptions controls entity cards in a business search.
type BusinessEntityOptions struct {
	Enable        *bool `json:"enable,omitempty"`
	Count         *int  `json:"count,omitempty"`
	MaxActivities *int  `json:"max_activities,omitempty"`
	MaxNews       *int  `json:"max_news,omitempty"`
}

// BusinessSearchParams is the request body of BusinessSearch.
type BusinessSearchParams struct {
	Query          string                 `json:"query"`
	Count          *int                   `json:"count,omitempty"`
	IncludeDomains []string               `json:"include_domains,omitempty"`
	ExcludeDomains []string               `json:"exclude_domains,omitempty"`
	TimeBasis      *string                `json:"time_basis,omitempty" enum:"auto,published,crawled"`
	TimeRange      *string                `json:"time_range,omitempty" enum:"day,week,month,year,d,w,m,y"`
	StartTime      *string                `json:"start_time,omitempty"`
	EndTime        *string                `json:"end_time,omitempty"`
	Language       []string               `json:"language,omitempty" enum:"ar,de,en,es,fr,hi,id,it,ja,ko,nl,pl,pt,ru,th,tr,vi,zh"`
	Highlight      *HighlightOptions      `json:"highlight,omitempty"`
	FullContent    *FullContentOptions    `json:"full_content,omitempty"`
	Entities       *BusinessEntityOptions `json:"entities,omitempty"`
}

// BusinessImage is an image attached to a business result.
type BusinessImage struct {
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
}

// BusinessSearchResult is one web result of a business search.
type BusinessSearchResult struct {
	Title           string          `json:"title,omitempty"`
	URL             string          `json:"url,omitempty"`
	Highlight       string          `json:"highlight,omitempty"`
	FullContent     string          `json:"full_content,omitempty"`
	Authors         string          `json:"authors,omitempty"`
	TimePublished   string          `json:"time_published,omitempty"`
	TimeLastCrawled string          `json:"time_last_crawled,omitempty"`
	Favicon         string          `json:"favicon,omitempty"`
	CoverImage      *BusinessImage  `json:"cover_image,omitempty"`
	Images          []BusinessImage `json:"images,omitempty"`
}

// BusinessNewsItem is a news article attached to an entity.
type BusinessNewsItem struct {
	Title           string          `json:"title,omitempty"`
	URL             string          `json:"url,omitempty"`
	Highlight       string          `json:"highlight,omitempty"`
	FullContent     string          `json:"full_content,omitempty"`
	TimePublished   string          `json:"time_published,omitempty"`
	TimeLastCrawled string          `json:"time_last_crawled,omitempty"`
	Favicon         string          `json:"favicon,omitempty"`
	CoverImage      *BusinessImage  `json:"cover_image,omitempty"`
	Images          []BusinessImage `json:"images,omitempty"`
}

// BusinessEntityIdentifiers are the external identifiers of a company.
type BusinessEntityIdentifiers struct {
	Website     string `json:"website,omitempty"`
	LinkedInURL string `json:"linkedin_url,omitempty"`
	StockTicker string `json:"stock_ticker,omitempty"`
	SecCIK      string `json:"sec_cik,omitempty"`
}

// BusinessEntityAttributes are the descriptive attributes of a company.
type BusinessEntityAttributes struct {
	Industry      string `json:"industry,omitempty"`
	HQLocation    string `json:"hq_location,omitempty"`
	FoundedYear   int    `json:"founded_year,omitempty"`
	EmployeeRange string `json:"employee_range,omitempty"`
}

// BusinessFinancials is one reporting period of company financials.
type BusinessFinancials struct {
	Revenue     float64 `json:"revenue,omitempty"`
	NetIncome   float64 `json:"net_income,omitempty"`
	GrossProfit float64 `json:"gross_profit,omitempty"`
	Period      string  `json:"period,omitempty"`
	Currency    string  `json:"currency,omitempty"`
}

// BusinessStock is a stock quote.
type BusinessStock struct {
	Price               float64 `json:"price,omitempty"`
	TodaysChangePercent float64 `json:"todays_change_percent,omitempty"`
	Week52High          float64 `json:"week_52_high,omitempty"`
	Week52Low           float64 `json:"week_52_low,omitempty"`
	Date                string  `json:"date,omitempty"`
	Currency            string  `json:"currency,omitempty"`
}

// BusinessFunding is one funding round.
type BusinessFunding struct {
	Round         string  `json:"round,omitempty"`
	Amount        float64 `json:"amount,omitempty"`
	Date          string  `json:"date,omitempty"`
	LeadInvestors string  `json:"lead_investors,omitempty"`
	Currency      string  `json:"currency,omitempty"`
}

// BusinessWebTraffic is a web traffic estimate.
type BusinessWebTraffic struct {
	VisitsMonthly int    `json:"visits_monthly,omitempty"`
	Rank          int    `json:"rank,omitempty"`
	Period        string `json:"period,omitempty"`
}

// BusinessEntityMetrics groups the numeric data of a company.
type BusinessEntityMetrics struct {
	Financials []BusinessFinancials `json:"financials,omitempty"`
	Stock      *BusinessStock       `json:"stock,omitempty"`
	Funding    []BusinessFunding    `json:"funding,omitempty"`
	WebTraffic *BusinessWebTraffic  `json:"web_traffic,omitempty"`
}

// BusinessKeyPerson is a key person of a company.
type BusinessKeyPerson struct {
	Name  string `json:"name,omitempty"`
	Title string `json:"title,omitempty"`
}

// BusinessCareerEntry is one position in a person's career.
type BusinessCareerEntry struct {
	Organization string `json:"organization,omitempty"`
	Title        string `json:"title,omitempty"`
	Start        string `json:"start,omitempty"`
	End          string `json:"end,omitempty"`
}

// CompanyEntityLogo is the logo of a company.
type CompanyEntityLogo struct {
	URL string `json:"url,omitempty"`
}

// PersonEntityPhoto is the photo of a person.
type PersonEntityPhoto struct {
	URL string `json:"url,omitempty"`
}

// CompanyEntity is the company variant of BusinessEntity.
type CompanyEntity struct {
	Type        string                     `json:"type"`
	Name        string                     `json:"name,omitempty"`
	Summary     string                     `json:"summary,omitempty"`
	Aliases     []string                   `json:"aliases,omitempty"`
	Identifiers *BusinessEntityIdentifiers `json:"identifiers,omitempty"`
	Attributes  *BusinessEntityAttributes  `json:"attributes,omitempty"`
	Metrics     *BusinessEntityMetrics     `json:"metrics,omitempty"`
	KeyPeople   []BusinessKeyPerson        `json:"key_people,omitempty"`
	Logo        *CompanyEntityLogo         `json:"logo,omitempty"`
	Activities  []BusinessSearchResult     `json:"activities,omitempty"`
	News        []BusinessNewsItem         `json:"news,omitempty"`
}

// PersonEntity is the person variant of BusinessEntity.
type PersonEntity struct {
	Type            string                 `json:"type"`
	Name            string                 `json:"name,omitempty"`
	Summary         string                 `json:"summary,omitempty"`
	LinkedInURL     string                 `json:"linkedin_url,omitempty"`
	CurrentPosition string                 `json:"current_position,omitempty"`
	Career          []BusinessCareerEntry  `json:"career,omitempty"`
	Photo           *PersonEntityPhoto     `json:"photo,omitempty"`
	Activities      []BusinessSearchResult `json:"activities,omitempty"`
	News            []BusinessNewsItem     `json:"news,omitempty"`
}

// BusinessEntity is a company or person card. Raw holds the original JSON for decoded entities; an
// unknown or missing type fills only Type and Raw, so new variants never break decoding.
type BusinessEntity struct {
	Type    string
	Company *CompanyEntity
	Person  *PersonEntity
	Raw     json.RawMessage
}

// UnmarshalJSON decodes the variant selected by "type" and keeps a copy of the input in Raw.
func (e *BusinessEntity) UnmarshalJSON(data []byte) error {
	*e = BusinessEntity{Raw: append(json.RawMessage(nil), data...)}
	var probe struct {
		Type json.RawMessage `json:"type"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return nil
	}
	var typ string
	if json.Unmarshal(probe.Type, &typ) != nil {
		return nil
	}
	e.Type = typ
	switch typ {
	case "company":
		e.Company = new(CompanyEntity)
		return json.Unmarshal(data, e.Company)
	case "person":
		e.Person = new(PersonEntity)
		return json.Unmarshal(data, e.Person)
	}
	return nil
}

// MarshalJSON writes Raw when present, so a decoded entity marshals back to the API's JSON.
func (e BusinessEntity) MarshalJSON() ([]byte, error) {
	switch {
	case len(e.Raw) > 0:
		return e.Raw, nil
	case e.Company != nil:
		return json.Marshal(e.Company)
	case e.Person != nil:
		return json.Marshal(e.Person)
	}
	return []byte("null"), nil
}

// BusinessSearchData is the data object of a business search response.
type BusinessSearchData struct {
	Query    string                 `json:"query,omitempty"`
	Results  []BusinessSearchResult `json:"results,omitempty"`
	Entities []BusinessEntity       `json:"entities,omitempty"`
}

// BusinessSearchUsage reports the billable work of a business search call.
type BusinessSearchUsage struct {
	NumSearchQueries      int `json:"num_search_queries,omitempty"`
	FullContentExtraCount int `json:"full_content_extra_count,omitempty"`
}

// BusinessSearchMeta is the meta object of a business search response.
type BusinessSearchMeta struct {
	Usage   *BusinessSearchUsage `json:"usage,omitempty"`
	Latency float64              `json:"latency,omitempty"`
	Warning string               `json:"warning,omitempty"`
}

// BusinessSearchResponse is the response of BusinessSearch.
type BusinessSearchResponse struct {
	Code      json.RawMessage     `json:"code,omitempty"`
	Msg       *string             `json:"msg,omitempty"`
	RequestID string              `json:"request_id"`
	Data      *BusinessSearchData `json:"data,omitempty"`
	Meta      *BusinessSearchMeta `json:"meta,omitempty"`

	raw []byte
}

// RawJSON returns the response body exactly as received, including fields this SDK does not model.
func (r *BusinessSearchResponse) RawJSON() []byte { return r.raw }
