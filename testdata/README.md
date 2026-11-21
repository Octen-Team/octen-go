# testdata

## openapi.json

Snapshot of the public Octen OpenAPI spec.

- Source: https://docs.octen.ai/api-reference/openapi.json
- Downloaded: 2026-11-17, copied byte-for-byte from the TypeScript SDK's snapshot of the same day
- Extended: 2026-11-21, the `/search-premier` and `/design-search` operations and the `Design*` schemas were copied in from the live spec of that day; the rest of the file was left as it was
- One edit: the `streamContentDelta` example in `/v1/messages` carried a Chinese weather sentence (`delta.text`); it was translated to English when the snapshot was taken (this repository is English-only). Re-apply that change after a re-download.

The contract test (`contract_test.go`) compares the request and response types of the ten endpoints against this file.
The weekly spec-drift workflow (`internal/tools/specdrift`) compares it with the live spec and reports drift, but never overwrites it.
Update it by hand: download it again, update the date above, then fix the types or `known_diffs.json`.

## known_diffs.json

Fields the SDK has but the snapshot lacks. Each entry is `{ "path": "<SchemaName>.<dotted.path>", "reason": "..." }`
and must carry a non-empty reason. Array items are written `field[]`, union variants `field[]{tag}`.
An entry that no longer matches an SDK-only field fails the contract test and must be removed.

## error_cases.json

Shared with the TypeScript SDK, which holds an identical copy: each case is an HTTP status, headers and body,
and the expected outcome: success, or the error class, code, message, request ID and Retry-After. `envelope_test.go` runs every case
against the 2xx success judgement and error extraction. Change both copies together.
