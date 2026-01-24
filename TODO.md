# TODO - SEND-68 Test Coverage

## Completed
- [x] TestConsensus_AllProvidersError - all providers return errors (initial)
- [x] TestConsensus_EmptyBlocksFromAllProviders - empty valid consensus (initial)
- [x] TestHashBlocks_DifferentDataProducesDifferentHashes - hash collision resistance (initial)
- [x] TestReceiptValidator_Enabled - accessor test (initial)
- [x] TestReceiptValidator_FetchReceiptHash_Disabled - disabled path (initial)
- [x] TestValidateReceipts_MetricsIncrement - metrics with mismatch (initial)
- [x] TestValidateReceipts_NilMetrics - nil metrics safety (initial)
- [x] TestAuditor_RunContextCancellation - context cancellation exits Run() (initial)
- [x] TestAuditor_Backpressure - inFlight limit respected (initial)
- [x] TestAuditor_VerifyTaskNotFound - error when task missing (initial)
- [x] TestAuditor_CheckEmptyQueue - empty queue handling (initial)
- [x] TestRepairDryRun - dry_run returns count without modifying (initial)
- [x] TestRepairStatusFilter - list filtering by status (initial)
- [x] TestConsensus_MaxAttemptsExhaustion - max consensus attempts (1000) exhaustion (iteration 1)
- [x] TestReceiptValidator_ValidateWithMockClient - full Validate() with mock RPC (iteration 1)
- [x] TestReceiptValidator_ValidateRPCError - RPC error propagation (iteration 1)
- [x] TestAuditor_EscalationThresholdConsensus - escalation threshold logic (iteration 1)
- [x] TestRepairReorgRetryExhaustion - reorg retry exhaustion (maxReorgRetries=10) (iteration 1)
- [x] TestRepairConsensusVsSingleProviderMode - consensus vs single provider mode selection (iteration 1)

## In Progress

## Pending

## Blocked

## Notes
- All SEND-68 test coverage gaps have been addressed
- Phase 1 (consensus): maxConsensusAttempts=1000 test added
- Phase 2 (receipt): Validate() with actual mock RPC added (not just validateReceipts helper)
- Phase 3 (audit): escalation fallback path at audit.go:302-308 tested
- Phase 4 (repair): reorg retry and mode selection tests added
- All tests pass: `go test -p 1 ./shovel`
- Build passes: `go build ./...`
- Dependencies tidy: `go mod tidy && git diff --exit-code go.mod go.sum`
