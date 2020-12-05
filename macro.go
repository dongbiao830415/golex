package main

import (
	"fmt"
	"unicode"
	"unsafe"
)

type stringList []string

func (l *stringList) String() string {
	return fmt.Sprintf("%s", *l)
}

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func (l stringList) Have(d []byte) bool {
	for _, s := range l {
		if s == bytes2str(d) {
			return true
		}
	}
	return false
}

func (l stringList) Size() int {
	return len(l)
}

type Define struct {
	stringList
}

func str2bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func ISSPACE(c byte) bool {
	return unicode.IsSpace(rune(c))
}

//判断字符变量c是否为字母或数字
func ISALNUM(c byte) bool {
	return unicode.IsLetter(rune(c)) || unicode.IsNumber(rune(c))
}

//判断字符ch是否为英文字母
func ISALPHA(c byte) bool {
	return unicode.IsLetter(rune(c))
}

func NegLikeC(i int) int {
	if i == 0 {
		return 1

	} else {
		return 0
	}
}

/* The text in the input is part of the argument to an %ifdef or %ifndef.
** Evaluate the text as a boolean expression.  Return true or false.
 */
//返回条件是真，还是假，正常只返回0和1，负数出了问题
func (self *Define) eval_preprocessor_boolean(z []byte, lineno int) (int, error) {
	neg := false
	var res int
	okTerm := true
	var i int
	empyt := false
	for ; i < len(z); i++ {
		if ISSPACE(z[i]) {
			//去掉前面的空格
			continue
		}
		empyt = true
		if z[i] == '!' {
			if !okTerm {
				goto pp_syntax_error
			}
			neg = !neg
			continue
		}
		if z[i] == '|' && z[i+1] == '|' {
			if okTerm {
				goto pp_syntax_error
			}
			if res != 0 { //结果已经为真
				return 1, nil
			}
			i++
			okTerm = true
			continue
		}
		if z[i] == '&' && z[i+1] == '&' {
			if okTerm {
				goto pp_syntax_error
			}
			if res == 0 { //结果已经为假
				return 0, nil
			}
			i++
			okTerm = true
			continue
		}
		if z[i] == '(' {
			if !okTerm {
				goto pp_syntax_error
			}
			k := i + 1
			n := 1
			for ; k < len(z); k++ {
				if z[k] == ')' {
					n--
					if n == 0 {
						//()中间，这里面还可能包含括号
						res, _ = self.eval_preprocessor_boolean(z[i+1:k], -1)
						if res < 0 {
							i = i - res
							goto pp_syntax_error
						}
						i = k
						break
					}

				} else if z[k] == '(' {
					n++ //括号嵌套
				}
			}
			if k >= len(z) {
				i = k
				goto pp_syntax_error
			}
			if neg {
				res = NegLikeC(res)
				neg = false
			}
			okTerm = false
			continue
		}
		if z[i] == '_' || ISALPHA(z[i]) { //下划线或者英文字母开头
			if !okTerm {
				goto pp_syntax_error
			}
			k := i + 1
			//C中z是有一个 \0 作为结束的
			for ; k < len(z) && (ISALNUM(z[k]) || z[k] == '_'); k++ {
			}
			res = 0
			if self.Have(z[i:k]) {
				res = 1 //条件为真
			}
			i = k - 1 //指向条件最后一个字符
			if neg {
				res = NegLikeC(res)
				neg = false
			}
			okTerm = false
			continue
		}
		goto pp_syntax_error
	}
	if !empyt {
		goto pp_syntax_error
	}
	return res, nil

pp_syntax_error:
	if lineno > 0 {
		//i+1表示输出字符的数量
		return 0, fmt.Errorf("%%if syntax error on line %d. [%.*s] <-- syntax error here", lineno, i+1, z)

	} else {
		return -(i + 1), nil //错误字符的长度
	}
}

func (self *Define) preprocess_input(z []byte) error {
	var err error
	var i, j int
	var exclude int //>0 表示这一块都要清理
	var start int
	lineno := 1
	start_lineno := 1
	for i = 0; i < len(z); i++ {
		if z[i] == '\n' {
			lineno++
		}
		if z[i] != '%' || (i > 0 && z[i-1] != '\n') {
			continue
		}
		//找到 % 开头的关键字
		if bytes2str(z[i:i+6]) == "%endif" && ISSPACE(z[i+6]) {
			if exclude != 0 {
				exclude--
				if exclude == 0 {
					for j = start; j < i; j++ {
						if z[j] != '\n' {
							z[j] = ' '
						}
					}
				}
			}
			for j = i; j < len(z) && z[j] != '\n'; j++ {
				z[j] = ' '
			}

		} else if bytes2str(z[i:i+5]) == "%else" && ISSPACE(z[i+5]) {
			if exclude == 1 {
				exclude = 0
				for j = start; j < i; j++ {
					if z[j] != '\n' {
						z[j] = ' '
					}
				}

			} else if exclude == 0 {
				exclude = 1
				start = i
				start_lineno = lineno
			}
			for j = i; j < len(z) && z[j] != '\n'; j++ {
				z[j] = ' '
			}

		} else if bytes2str(z[i:i+7]) == "%ifdef " ||
			bytes2str(z[i:i+4]) == "%if " ||
			bytes2str(z[i:i+8]) == "%ifndef " {
			if exclude != 0 {
				exclude++

			} else {
				//关键字后面的空格
				for j = i; j < len(z) && !ISSPACE(z[j]); j++ {
				}
				iBool := j          //%ifndef后面的空格
				isNot := (j == i+7) //是否为 %ifndef 关键字
				//找出 %ifndef 后面的关键字，包含空格
				for ; j < len(z) && z[j] != '\n'; j++ {
					if z[j] == '\r' {
						z[j] = ' '
					}
				}
				//应该是对 %ifndef 后面条件的判断
				if exclude, err = self.eval_preprocessor_boolean(z[iBool:j], lineno); err != nil {
					return err
				}
				if !isNot {
					exclude = NegLikeC(exclude)
				}
				if exclude != 0 {
					start = i
					start_lineno = lineno
				}
			}
			for j = i; j < len(z) && z[j] != '\n'; j++ {
				z[j] = ' '
			}
		}
	}
	if exclude != 0 {
		return fmt.Errorf("unterminated %%ifdef starting on line %d", start_lineno)
	}
	return nil
}
