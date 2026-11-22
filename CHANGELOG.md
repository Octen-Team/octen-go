# Changelog

## Unreleased

- `Extract.Extract` (`POST /extract`) with `ExtractParams` and `*ExtractResponse`, checked against the public OpenAPI spec. Results are per URL (`Status`, `ErrorMessage`, `ResolvedMode`), and `Meta.Usage.SuccessfulByMode` gives what the call is billed for. `IncludeLinks: &octen.ExtractLinksOptions{}` is sent as `{}` and uses the server defaults.
- Local checks for Extract, reported as `*ValidationError` before anything is sent: `URLs` must hold 1 to 20 URLs, each non-blank and at most 2048 characters (Unicode code points). Other ranges are left to the server, as for the other endpoints.
- Extract's default timeout is its per-URL `Timeout` (clamped to 1 to 60 s, default 30) plus 90 s, so 120 s by default; `WithRequestTimeout` or an explicit `WithTimeout` takes precedence, as for `BroadSearch`, and a shorter context deadline still wins. Retries follow the existing policy; the README states what a retry can cost on a per-URL billed call.
- The contract test and the spec-drift check cover Extract; spec-drift also compares the `urls` limits (item bounds, item lengths and the description that states them) and the per-URL `timeout` default and range, which the SDK's checks and default timeout rely on.

## 0.2.0

- `Search.SearchPremier` for Web Search Premier (`POST /search-premier`). It takes `SearchParams` and returns `*SearchResponse`, like `Search`.
- `Search.ImageSearch` (`POST /image-search`) with `ImageSearchParams`; `Inputs` must hold exactly one text input or one image input, given as a public `URL` or base64 `Data`.
- `Search.VideoSearch` (`POST /video-search`) with `VideoSearchParams`; `Inputs` must hold exactly one text input. Video Search is an invite-only beta.
- `Search.DesignSearch` (`POST /design-search`) with `DesignSearchParams`: design references as image results (type, screenshot, group, design facts, snippet) plus web results. `Query` is optional when `Images.GroupID` is set, which returns the images of that page or deck in order.
- Local checks for the new endpoints, reported as `*ValidationError` before anything is sent: Image and Video Search need exactly one input of a supported `Type` (`text` or `image`; `text` only for video), a text input needs non-blank `Data` and no `URL`, and an image input needs exactly one of a non-blank `URL` or `Data`; Design Search needs a non-blank `Query` or `Images.GroupID`.
- The contract test and the spec-drift check cover the four new endpoints.

## 0.1.0

Initial release.

- `octen.NewClient` with `Search.Search`, `Search.BroadSearch`, `Search.NewsSearch`, `Search.BusinessSearch`, `Search.PulseSearch` and `PlaceSearch.Search`.
- Request and response types for the six endpoints, checked against the public OpenAPI spec by a contract test; `RawJSON()` on every response returns the body as received.
- `BusinessEntity` decodes `company` and `person` entities and keeps `Type` and `Raw` for any other `type`; `json.Marshal` writes `Raw` back, so a decoded response survives a marshal and unmarshal round trip.
- Error types `*APIError`, `*TimeoutError`, `*ConnectionError` and `*ValidationError`, and the sentinels `ErrAuthentication`, `ErrPermissionDenied`, `ErrBadRequest`, `ErrRateLimit` and `ErrServer` for `errors.Is`. `*TimeoutError` also matches `context.DeadlineExceeded`.
- A 2xx response is a failure when the body is not a non-empty JSON object or its `code` is a non-zero number; a successful body that does not decode into the typed response returns an `*APIError` starting `decode response:`.
- Automatic retries for HTTP 408, 429, 500, 502, 503 and 504, and for network errors that happen before the request is sent (dial and DNS failures); SDK timeouts, cancellation of your context and later network errors are not retried. Other 5xx statuses fail at once.
- Backoff of `1 s * 2^(n-1)` with up to 25% jitter, at most 60 s; `Retry-After` is honoured on 429 and 503 only, and a value above 60 s stops retrying.
- Default timeout of 30 s per attempt and 120 s for `BroadSearch`; precedence is `WithRequestTimeout`, an explicit `WithTimeout`, then the endpoint default. The timeout includes reading the response body.
- Client options `WithAPIKey` (falls back to `OCTEN_API_KEY`), `WithBaseURL`, `WithTimeout`, `WithMaxRetries`, `WithHeader` and `WithHTTPClient`; request options `WithRequestTimeout` and `WithRequestHeader`. `WithHTTPClient` works on a copy of the client you pass in.
- Local checks reject a blank query, `NaN` and infinite numbers, `x-api-key` in custom headers, invalid header names, header values and API keys with a control character other than tab, and a base URL with credentials, a query, a fragment or a port outside 1-65535.
- Redirects are not followed, so the API key is only ever sent to the configured base URL.
- Pointer helpers `Ptr`, `Int`, `String`, `Bool` and `Float64` for optional request fields.
- No dependencies outside the standard library; requires Go 1.22 or newer.
