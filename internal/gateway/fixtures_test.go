package gateway

// pngBytes is an image's bytes as the tests send one: a PNG's signature,
// which is all a sniffer reads.
var pngBytes = []byte("\x89PNG\r\n\x1a\n fake")
