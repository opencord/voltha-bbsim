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
	"time"

	lkh "github.com/gfremex/logrus-kafka-hook"
	log "github.com/sirupsen/logrus"
)

var (
	myLogger *log.Entry
)

// Setup logger
func Setup(kafkaBroker string, level string) {

	logger := log.New()
	//logger.SetReportCaller(true)
	myLogger = logger.WithField("topics", []string{"bbsim.log"})

	var logLevel = log.DebugLevel
	switch level {
	case "TRACE":
		logLevel = log.TraceLevel
	case "INFO":
		logLevel = log.InfoLevel
	case "WARN":
		logLevel = log.WarnLevel
	case "ERROR":
		logLevel = log.ErrorLevel
	default:
		logLevel = log.DebugLevel
	}
	logger.Println("Setting Log Level ", logLevel)
	logger.SetLevel(logLevel)

	if len(kafkaBroker) > 0 {
		myLogger.Debug("Setting up kafka integration")
		hook, err := lkh.NewKafkaHook(
			"kh",
			[]log.Level{log.DebugLevel, log.InfoLevel, log.WarnLevel, log.ErrorLevel},
			&log.JSONFormatter{
				TimestampFormat: time.RFC3339Nano,
				FieldMap: log.FieldMap{
					log.FieldKeyTime:  "@timestamp",
					log.FieldKeyLevel: "levelname",
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

// GetLogger return logger instance
func GetLogger() *log.Entry {
	return myLogger
}

// WithField logs message with specified field
func WithField(key string, value interface{}) *log.Entry {
	return myLogger.WithField(key, value)
}

// WithFields logs message with multiple fields
func WithFields(fields log.Fields) *log.Entry {
	return myLogger.WithFields(fields)
}

// Panic logs with log level panic
func Panic(msg string, args ...interface{}) {
	myLogger.Panicf(msg, args...)
}

// Fatal logs woth loge level fatal
func Fatal(msg string, args ...interface{}) {
	myLogger.Fatalf(msg, args...)
}

// Error logs with log level error
func Error(msg string, args ...interface{}) {
	myLogger.Errorf(msg, args...)
}

// Warn logs with log level warn
func Warn(msg string, args ...interface{}) {
	myLogger.Warnf(msg, args...)
}

// Info logs with log level info
func Info(msg string, args ...interface{}) {
	myLogger.Infof(msg, args...)
}

// Debug logs with log level debug
func Debug(msg string, args ...interface{}) {
	myLogger.Debugf(msg, args...)
}
