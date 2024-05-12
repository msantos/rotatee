package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	version = "0.1.0"
)

type State struct {
	prefix  string
	dir     string
	format  string
	maxsize int
	cur     int
	ignore  bool
	errMode func(error) error

	sigch chan os.Signal
}

func usage() {
	fmt.Fprintf(os.Stderr, `%s v%s
Usage: %s [<option>] [<fileprefix>]

tee(1) with file rotation.

Examples:

  # writes output to files in the current directory prefixed with "stdout"
  rotatee

  # writes output to files in /tmp prefixed with "output"
  rotatee --dir=/tmp output

Options:

`, path.Base(os.Args[0]), version, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	dir := flag.String("dir", ".", "output directory")
	maxsize := flag.Int("maxsize", 100, "max file size (MiB)")
	format := flag.String("format", time.RFC3339+".log", "timestamp")
	ignore := flag.Bool("ignore", false, "ignore SIGTERM")
	outputError := flag.String("output-error", "sigpipe", "set behavior on write error (warn, warn-nopipe, exit, exit-nopipe)")

	flag.Usage = func() { usage() }
	flag.Parse()

	prefix := "stdout"

	if flag.NArg() > 0 {
		prefix = flag.Arg(0)
	}

	state := &State{
		prefix:  prefix,
		format:  *format,
		dir:     *dir,
		maxsize: *maxsize * 1024 * 1024,
		ignore:  *ignore,
		sigch:   make(chan os.Signal, 1),
		errMode: mode(*outputError),
	}

	if err := os.MkdirAll(*dir, 0700); state.errMode(err) != nil {
		log.Fatalln(err)
	}

	if err := state.initialize(); err != nil {
		log.Fatalln(err)
	}

	go state.signal()

	if err := state.run(); err != nil {
		log.Fatalln(err)
	}
}

func (state *State) initialize() error {
	glob := filepath.Join(state.dir, "*.rotatee")
	matches, err := filepath.Glob(glob)
	if err != nil {
		return fmt.Errorf("%s:%w", glob, err)
	}

	for _, v := range matches {
		if err := state.rename(v); state.errMode(err) != nil {
			return err
		}
	}

	return nil
}

func (state *State) signal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGPIPE)

	for {
		sig := <-ch
		switch sig {
		case syscall.SIGPIPE:
			continue
		case syscall.SIGHUP:
		case syscall.SIGTERM:
			if state.ignore {
				continue
			}
		default:
			log.Printf("unhandled signal received: %s\n", sig)
			continue
		}
		state.sigch <- sig
	}
}

func (state *State) path(t time.Time) string {
	ts := t.Format(state.format)
	return filepath.Join(state.dir, state.prefix+"."+ts+".rotatee")
}

func (state *State) rename(oldpath string) error {
	newpath := strings.TrimSuffix(oldpath, ".rotatee")
	return os.Rename(oldpath, newpath)
}

func (state *State) run() error {
	path := state.path(time.Now())
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if state.errMode(err) != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	defer func() {
		_ = state.rename(path)
	}()

	rotate := false

	stdin := bufio.NewScanner(os.Stdin)
	for stdin.Scan() {
		s := stdin.Text()
		if _, err := fmt.Println(s); state.errMode(err) != nil {
			return err
		}

		// Include newline in count.
		if len(s)+1+state.cur > state.maxsize {
			rotate = true
		}

		if rotate {
			rotate = false
			state.cur = 0

			if err := f.Close(); state.errMode(err) != nil {
				return fmt.Errorf("%s: %w", path, err)
			}

			if err := state.rename(path); state.errMode(err) != nil {
				return fmt.Errorf("%s: %w", path, err)
			}

			path = state.path(time.Now())
			f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
			if state.errMode(err) != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}

		state.cur += len(s) + 1 // newline

		if _, err := fmt.Fprintln(f, s); state.errMode(err) != nil {
			return err
		}

		select {
		case sig := <-state.sigch:
			switch sig {
			case syscall.SIGTERM:
				return nil
			case syscall.SIGHUP:
				rotate = true
			}
		default:
		}
	}

	return stdin.Err()
}

func mode(s string) func(error) error {
	switch s {
	case "ignore":
		return modeIgnore
	case "warn":
		return modeWarn
	case "warn-nopipe":
		return modeWarnNoPipe
	case "exit":
		return modeExit
	case "exit-nopipe":
		return modeExitNoPipe
	default:
		return modeSigPipe
	}
}

func modeIgnore(err error) error {
	return nil
}

func modeWarn(err error) error {
	if err == nil || errors.Is(err, os.ErrInvalid) {
		return nil
	}
	log.Println(err)
	return nil
}

func modeWarnNoPipe(err error) error {
	if err == nil || errors.Is(err, os.ErrInvalid) || errors.Is(err, syscall.EPIPE) {
		return nil
	}
	log.Println(err)
	return nil
}

func modeExit(err error) error {
	return err
}

func modeExitNoPipe(err error) error {
	if err == nil || errors.Is(err, os.ErrInvalid) || errors.Is(err, syscall.EPIPE) {
		return nil
	}
	return err
}

func modeSigPipe(err error) error {
	if err == nil || errors.Is(err, os.ErrInvalid) {
		return nil
	}
	if errors.Is(err, syscall.EPIPE) {
		return err
	}
	log.Println(err)
	return nil
}
