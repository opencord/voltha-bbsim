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
	"fmt"

	lkh "github.com/gfremex/logrus-kafka-hook"
	log "github.com/sirupsen/logrus"
)

var (
	myLogger *log.Entry
)

func Setup(kafkaBroker string, level string) {

	logger := log.New()
	//logger.SetReportCaller(true)
	myLogger = logger.WithField("topics", []string{"bbsim.log"})

	// TODO make this configurable via cli arg
	if level == "DEBUG" {
		logger.SetLevel(log.DebugLevel)
	}

	if len(kafkaBroker) > 0 {
		myLogger.Debug("Setting up kafka integration")
		hook, err := lkh.NewKafkaHook(
			"kh",
			[]log.Level{log.DebugLevel, log.InfoLevel, log.WarnLevel, log.ErrorLevel},
			&log.JSONFormatter{
				FieldMap: log.FieldMap{
					log.FieldKeyTime:  "@timestamp",
					log.FieldKeyLevel: "level",
					log.FieldKeyMsg:   "message",
				},
			},
			[]string{kafkaBroker},
		)

		if err != nil {
			myLogger.Error(err)
		}

		logger.Hooks.Add(hook)

	}

	myLogger.WithField("kafkaBroker", kafkaBroker).Debug("Logger setup done")
}

func GetLogger() *log.Entry {
	return myLogger
}

func WithField(key string, value interface{}) *log.Entry {
	return myLogger.WithField(key, value)
}

func WithFields(fields log.Fields) *log.Entry {
	return myLogger.WithFields(fields)
}

func Panic(msg string, args ...interface{}) {
	myLogger.Panic(fmt.Sprintf(msg, args...))
}

func Fatal(msg string, args ...interface{}) {
	myLogger.Fatal(fmt.Sprintf(msg, args...))
}

func Error(msg string, args ...interface{}) {
	myLogger.Error(fmt.Sprintf(msg, args...))
}

func Warn(msg string, args ...interface{}) {
	myLogger.Warn(fmt.Sprintf(msg, args...))
}

func Info(msg string, args ...interface{}) {
	myLogger.Info(fmt.Sprintf(msg, args...))
}

func Debug(msg string, args ...interface{}) {
	myLogger.Debug(fmt.Sprintf(msg, args...))
}
