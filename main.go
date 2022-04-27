package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

var verbose bool

const (
	EnvVerbose      = "SELFEXTRACT_VERBOSE"
	EnvDir          = "SELFEXTRACT_DIR"
	EnvStartup      = "SELFEXTRACT_STARTUP"
	EnvExtractOnly  = "SELFEXTRACT_EXTRACT_ONLY"
	EnvGraceTimeout = "SELFEXTRACT_GRACE_TIMEOUT"
)

func init() {
	verbose = isTruthy(os.Getenv(EnvVerbose))
}

func main() {
	stub, payload, key := readSelf()

	if payload != nil {
		extract(payload, key)
		return
	}

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "%s [OPTION...] FILE ...\n", os.Args[0])
		flag.PrintDefaults()
	}
	createName := flag.String("f", "selfextract.out", "name of the archive to create")
	changeDir := flag.String("C", ".", "change dir before archiving files, only affects input files")
	verboseFlg := flag.Bool("v", false, "verbose output")
	flag.Parse()
	verbose = verbose || *verboseFlg

	create(stub, key, *createName, flag.Args(), *changeDir)
}

func debug(v ...interface{}) {
	if verbose {
		v = append([]interface{}{"selfextract:"}, v...)
		log.Println(v...)
	}
}

func die(v ...interface{}) {
	v = append([]interface{}{"selfextract: FATAL:"}, v...)
	log.Fatalln(v...)
}

func isTruthy(s string) bool {
	switch strings.ToLower(s) {
	case "y", "yes", "true", "1":
		return true
	default:
		return false
	}
}

func generateBoundary() []byte {
	h := sha512.Sum512([]byte("boundary"))
	return h[:]
}

const keyLength = 16

func generateRandomKey() []byte {
	buf := make([]byte, keyLength)
	_, err := rand.Read(buf)
	if err != nil {
		die("generating random key:", err)
	}
	return buf
}

// maxBoundaryOffset is the offset at which we stop looking for a boundary,
// it's just a failsafe mechanism against big, corrupted archives. We set it to
// a value much bigger than the expected size of the compiled stub.
const maxBoundaryOffset = 100e6 // 100 MB

func readSelf() ([]byte, io.ReadCloser, []byte) {
	t := time.Now()
	self, err := os.Open(os.Args[0])
	if err != nil {
		die("opening itself:", err)
	}
	debug("opened itself in", time.Since(t))

	t = time.Now()
	buf := make([]byte, maxBoundaryOffset+keyLength)
	n, err := self.Read(buf)
	var bufFull bool
	if err == io.EOF {
		bufFull = true
	} else if err != nil {
		die("reading itself:", err)
	}
	buf = buf[:n]
	debug("read itself in", time.Since(t))

	boundary := generateBoundary()
	t = time.Now()
	bdyOff := bytes.Index(buf[:maxBoundaryOffset], boundary)
	debug("boundary search completed in", time.Since(t))

	if bdyOff == -1 {
		if bufFull {
			die("boundary not found before byte", maxBoundaryOffset)
		}
		debug("no boundary")
		self.Close()
		return buf, nil, nil
	}
	debug("boundary found at", bdyOff)

	keyOff := bdyOff + len(boundary)
	payloadOff := keyOff + keyLength
	key := buf[keyOff:payloadOff]
	debug("key:", hex.EncodeToString(key))

	_, err = self.Seek(int64(payloadOff), os.SEEK_SET)
	if err != nil {
		die("seeking to start of payload:", err)
	}
	buf = buf[:bdyOff]

	return buf, self, key
}
