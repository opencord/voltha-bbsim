/*
 * Copyright 2018-present Open Networking Foundation

 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package logger

import (
	"strings"
	"log"
)

func Error(s string, opts ...interface{}){
	trimmed := strings.TrimRight(s, "\n")
	if len(opts) == 0{
		log.Printf("[ERROR]:%s\n", trimmed)
	}else{
		fmt := "[ERROR]:" + trimmed + "\n"
		log.Printf(fmt, opts...)
	}
}

func Debug(s string, opts ...interface{}){
	trimmed := strings.TrimRight(s, "\n")
	if len(opts) == 0{
		log.Printf("[DEBUG]:%s\n", trimmed)
	}else{
		fmt := "[DEBUG]:" + trimmed + "\n"
		log.Printf(fmt, opts...)
	}
}

func Info(s string, opts ...interface{}){
	trimmed := strings.TrimRight(s, "\n")
	if len(opts) == 0{
		log.Printf("[INFO]:%s\n", trimmed)
	}else{
		fmt := "[INFO]:" + trimmed + "\n"
		log.Printf(fmt, opts...)
	}
}