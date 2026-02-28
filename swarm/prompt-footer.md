
---
## MANDATORY: Test-Driven Development Process
1. Read SPEC.md in the repo root before writing any code
2. Write FAILING tests FIRST that define expected behavior from the spec
3. Implement minimum code to pass tests
4. Quality gates before commit:
   - `go build ./...` passes
   - `go test ./... -count=1` — all tests pass
   - `go vet ./...` — clean
   - Coverage meets target specified in the ticket
5. Commit message format: `feat(TICKET_ID): description`
6. Include in commit body: `Tests: X passing, Coverage: Y%`
7. Do NOT commit if any test is failing
