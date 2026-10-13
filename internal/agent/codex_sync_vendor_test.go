package agent

// codexUsedUpReal is codexUsedUp as the sync runs it: TestMain stands in for
// it (a test must never ask OpenAI whether the account is out), and a package
// variable is initialized before TestMain runs, so this keeps the real one
// for the test below to check.
var codexUsedUpReal = codexUsedUp
