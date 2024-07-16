# SYNOPSIS

rotatee [*options*] [*file prefix*]

# DESCRIPTION

`rotatee` is `tee(1)` with file rotation:
* stdin is written to stdout
* stdin is also written to a file with a timestamp
* if the next line of input exceeds the maximum configured number of
  bytes, the input is written to a new file
* if SIGTERM is received, `rotatee` will exit after writing the next
  line of input

# BUILDING

```
go install codeberg.org/msantos/rotatee/cmd/rotatee@latest
```

## Source

To build a reproducible executable from the git repository:

```
cd cmd/rotatee
CGO_ENABLED=0 go build -C cmd/rotatee -trimpath -ldflags "-w"
```

# EXAMPLES

```
# writes output to files in  the current directory prefixed with "stdout"
rotatee

# writes output to files in /tmp prefixed with "output"
rotatee --dir=/tmp output
```

# SIGNALS

SIGHUP
: rotate log file

SIGTERM
: write next line of input and exit (use `--ignore` to disable)

# OPTIONS

dir *string*
: output directory (default ".")

format *string*
: timestamp format (default "2006-01-02T15:04:05Z07:00.log")

ignore
: ignore SIGTERM

maxsize *int*
: max file size (MiB) (default 100)

output-error *string*
: set behavior on write error (sigpipe, warn, warn-nopipe, exit, exit-nopipe) (default "sigpipe")

* warn: diagnose errors writing to any output
* warn-nopipe: diagnose errors writing to any output not a pipe
* exit: exit on error writing to any output
* exit-nopipe: exit on error writing to any output not a pipe
* sigpipe: exit on any error writing to a pipe, diagnose errors writing
  to any output not a pipe
* ignore: suppress and do not exit on errors
