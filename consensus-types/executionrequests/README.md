# Experimental full-exit requests

This package provides an exact 104-byte candidate record codec and SSZ public-key-root calculation using Qrysm's native fastssz hasher. Each record contains a 64-byte execution-authenticated source, an 8-byte little-endian validator index and a 32-byte key root. The outer request-type byte belongs to future versioned transport.

The accompanying `blocks.ProcessExecutionExitRequests` handler checks the complete container, works on a native state copy, compares the entire withdrawal recipient and key identity, consumes ineligible records, and delegates eligible exits to `InitiateValidatorExit`. Existing exits and requested exits share churn. Errors discard the state copy; callers must adopt the returned state only on success.

The handler is intentionally uncalled by the canonical block transition. Raw request bytes have no authority until a future fork binds them to execution-derived queue output and the EL header commitment. Integration requires full/blinded beacon containers, generated codecs, versioned Engine exchange, configured limits and placement after conventional operations. No network activation is included.

## Local checks

```sh
GOMAXPROCS=2 go test -p 2 ./consensus-types/executionrequests ./beacon-chain/core/blocks -run 'TestExecutionExit|TestDecodeExecutionExits' -count=1
GOMAXPROCS=2 go vet -p 2 ./consensus-types/executionrequests ./beacon-chain/core/blocks
GOMAXPROCS=2 go build -p 2 ./consensus-types/executionrequests ./beacon-chain/core/blocks
```

Tests use native BeaconState and scheduling, verify frozen nonzero SSZ vectors, and cover duplicates, mixed pending exits, ineligible prefixes, malformed input, cancellation and overflow atomicity. Signature processing, Engine exchange and network qualification remain separate checks.
