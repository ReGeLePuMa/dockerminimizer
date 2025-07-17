package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

var Log = logrus.New()

func InitLogger() {
	debug := os.Getenv("debug") == "true"
	Log.SetOutput(os.Stdout)
	if debug {
		Log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
			ForceColors:   true,
		})
		Log.SetReportCaller(true)
		Log.SetLevel(logrus.TraceLevel)
	} else {
		Log.SetFormatter(&logrus.TextFormatter{
			ForceColors:      true,
			FullTimestamp:    false,
			DisableQuote:     true,
			DisableSorting:   true,
			DisableTimestamp: true,
		})
		Log.SetReportCaller(false)
		Log.SetLevel(logrus.InfoLevel)
	}
}
