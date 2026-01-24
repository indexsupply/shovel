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
- [x] TestConsensus_MaxAttemptsExhaustion - configurable MaxAttempts exhaustion (iteration 2)
- [x] TestReceiptValidator_ValidateWithMockClient - full Validate() with mock RPC (iteration 1)
- [x] TestReceiptValidator_ValidateRPCError - RPC error propagation (iteration 1)
- [x] TestAuditor_EscalationThresholdConsensus - deterministic escalation threshold assertion (iteration 2)
- [x] TestRepairReorgRetryExhaustion - reorg retry exhaustion (maxReorgRetries=10) (iteration 1)
- [x] TestRepairConsensusVsSingleProviderMode - single provider mode paths (iteration 1)
- [x] TestRepairConsensusMode - actual consensus mode with real ConsensusEngine (iteration 2)

## In Progress

## Pending

## Blocked

## Notes
- All SEND-68 test coverage gaps have been addressed
- Iteration 2 fixes:
  - ISSUE-1: Made MaxAttempts configurable in config.Consensus, test now verifies exact error message after 5 attempts
  - ISSUE-2: Escalation test now deterministically asserts "healthy" status, not "retrying"
  - ISSUE-3: Added TestRepairConsensusMode to exercise consensus mode path (url="" + task.consensus != nil)
- All tests pass: `go test -p 1 ./shovel`
- Build passes: `go build ./...`
- Dependencies tidy: `go mod tidy && git diff --exit-code go.mod go.sum`
