package cli

import (
	"bufio"
	"os"
)

// stdin is read through one reader, so a second answer piped in is the
// second line and not lost to the first one's buffer.
var stdin = bufio.NewReader(os.Stdin)
