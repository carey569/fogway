package provider

// A backup carries providers.json's entries as they are stored, and puts
// them back on another machine (see internal/backup).

// handOver is HandOver: a var so a test can see it asked.
var handOver = HandOver
