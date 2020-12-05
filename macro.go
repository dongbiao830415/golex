package main

import (
	"fmt"
	"log"
	"unicode"
	"unsafe"
)

var (
	ifdef  = "%ifdef"
	ifndef = "%ifndef"
	endif  = "%endif"
)

type stringList []string

func (l *stringList) String() string {
	return fmt.Sprintf("%s", *l)
}
func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

var azDefine stringList

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
