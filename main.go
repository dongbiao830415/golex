// Copyright (c) 2014 The golex Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"unicode"
	"unsafe"

	"modernc.org/lex"
)

const (
	oFile = "lex.yy.go"
)

var (
	stdin  = bufio.NewReader(os.Stdin)
	stdout = bufio.NewWriter(os.Stdout)
	stderr = bufio.NewWriter(os.Stderr)
)

type renderer interface {
	render(srcname string, l *lex.L)
}

type writer interface {
	io.Writer
	wprintf(s string, args ...interface{}) (n int, err error)
}

type noRender struct {
	w io.Writer
}

func (r *noRender) Write(p []byte) (n int, err error) {
	return r.w.Write(p)
}

func (r *noRender) wprintf(s string, args ...interface{}) (n int, err error) {
	n, err = io.WriteString(r.w, fmt.Sprintf(s, args...))
	if err != nil {
		log.Fatal(err)
	}

	return
}

func q(c uint32) string {
	switch c {
	default:
		r := rune(c)
		if r >= 0 && r <= unicode.MaxRune {
			s := fmt.Sprintf("%q", string(r))
			return "'" + s[1:len(s)-1] + "'"
		}
		return ""
	case '\'':
		return "'\\''"
	case '"':
		return "'\"'"
	}
}

func main() {
	log.SetFlags(log.Flags() | log.Lshortfile)
	oflag := ""
	var dfaflag, hflag, tflag, vflag, nodfaopt, bits32, eflag bool
	var dflag string

	flag.BoolVar(&dfaflag, "DFA", false, "write DFA on stdout and quit")
	flag.BoolVar(&hflag, "h", false, "show help and exit")
	flag.StringVar(&oflag, "o", oFile, "lexer output")
	flag.BoolVar(&tflag, "t", false, "write scanner on stdout instead of "+oFile)
	flag.BoolVar(&vflag, "v", false, "write summary of scanner statistics to stderr")
	flag.BoolVar(&nodfaopt, "nodfaopt", false, "disable DFA optimization - don't use this for production code")
	flag.BoolVar(&eflag, "e", false, "preprocess only")
	flag.StringVar(&dflag, "D", "", "define an %ifdef macro.")
	//flag.BoolVar(&bits32, "32bit", false, "assume unicode rune lexer (partially implemented)")
	flag.Parse()
	if hflag || flag.NArg() > 1 {
		flag.Usage()
		fmt.Fprintf(stderr, "\n%s [-o out_name] [other_options] [in_name]\n", os.Args[0])
		fmt.Fprintln(stderr, "  If no in_name is given then read from stdin.")
		stderr.Flush()
		os.Exit(1)
	}

	var (
		lfile  *bufio.Reader // source .l
		gofile *bufio.Writer // dest .go
	)

	lname := flag.Arg(0)
	if lname == "" {
		lfile = stdin
	} else {
		defineList := strings.Split(dflag, ",")
		for _, v := range defineList {
			define := strings.TrimSpace(v)
			if define == "" {
				continue
			}
			azDefine = append(azDefine, define)
		}

		if len(azDefine) <= 0 {
			l, err := os.Open(lname)
			if err != nil {
				log.Fatal(err)
			}

			defer l.Close()
			lfile = bufio.NewReader(l)

		} else {
			b, err := ioutil.ReadFile(lname)
			if err != nil {
				log.Fatal(err)
			}
			preprocess_input(b)

			if eflag {
				if oflag == "" {
					oflag = oFile
				}
				efile := strings.TrimSuffix(oflag, ".go") + ".e"
				ioutil.WriteFile(efile, b, 0644)
			}

			fi := bytes.NewReader(b)
			lfile = bufio.NewReader(fi)
		}
	}

	l, err := lex.NewL(lname, lfile, nodfaopt, bits32)
	if err != nil {
		log.Fatal(err)
	}

	if dfaflag {
		fmt.Println(l.DfaString())
		os.Exit(1)
	}

	if tflag {
		gofile = stdout
	} else {
		if oflag == "" {
			oflag = oFile
		}
		g, err := os.Create(oflag)
		if err != nil {
			log.Fatal(err)
		}

		defer g.Close()
		gofile = bufio.NewWriter(g)
	}
	defer gofile.Flush()
	var buf bytes.Buffer
	renderGo{noRender{&buf}, map[int]bool{}}.render(lname, l)
	dst, err := format.Source(buf.Bytes())
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "%v\n", err)
		if _, err := gofile.Write(buf.Bytes()); err != nil {
			log.Fatal(err)
		}
	default:
		if _, err := gofile.Write(dst); err != nil {
			log.Fatal(err)
		}
	}

	if vflag {
		fmt.Fprintln(os.Stderr, l.String())
	}
}

var (
	ifdef  = "%ifdef"
	ifndef = "%ifndef"
	endif  = "%endif"
)

var azDefine []string

func str2bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func preprocess_input(z []byte) {
	var i, j, n, exclude, start int
	lineno := 1
	start_lineno := 1 // 第一层的%ifdef 和 %ifndef 的位置

	size := len(z)

	keyEqual := func(i int, des string) bool {
		if z[i] != '%' {
			return false
		}

		end := i + len(des)

		if bytes2str(z[i:end]) != des { //即使i也越界了，也不会core dump
			return false
		}

		if des == endif {
			if end < size && z[end] != '\n' && !unicode.IsSpace(rune(z[end])) {
				return false
			}

		} else if end >= size || !unicode.IsSpace(rune(z[end])) {
			return false
		}

		return true
	}

	idEqual := func(i int, des string) bool {
		end := i + len(des)
		if bytes2str(z[i:end]) != des {
			return false
		}
		return true
	}

	emptyLine := func(i int) {
		for j := i; j < size && z[j] != '\n'; j++ {
			z[j] = ' '
		}
	}

	for i = 0; i < size; i++ {
		if z[i] == '\n' {
			lineno++
		}
		if z[i] != '%' || (i > 0 && z[i-1] != '\n') {
			continue
		}

		//以上的操作如果是排除，则直接路过
		if keyEqual(i, endif) {
			if exclude > 0 {
				exclude--
				if exclude == 0 {
					for j = start; j < i; j++ {
						if z[j] != '\n' {
							z[j] = ' '
						}
					}
				}
			}
			emptyLine(i)

		} else if keyEqual(i, ifdef) || keyEqual(i, ifndef) {
			if exclude > 0 {
				//深度加1，这一行肯定是不能要了
				exclude++

			} else {
				for j = i + 7; unicode.IsSpace(rune(z[j])); j++ {
				}
				for n = 0; j+n < size && !unicode.IsSpace(rune(z[j+n])); n++ {
				}
				exclude = 1 //排除

				for _, v := range azDefine {
					if idEqual(j, v) {
						exclude = 0
						break
					}
				}

				if z[i+3] == 'n' {
					if exclude == 1 {
						exclude = 0

					} else {
						exclude = 1
					}
				}

				if exclude > 0 {
					start = i //开始的位置
					start_lineno = lineno
				}
			}
			emptyLine(i)
		}
	}

	if exclude > 0 {
		log.Fatalf("unterminated %%ifdef starting on line %d\n", start_lineno)
	}
}
