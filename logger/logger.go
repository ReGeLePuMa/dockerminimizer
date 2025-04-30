package logger

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

var Log = logrus.New()

func InitLogger() {
	Log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	if os.Getenv("debug") != "true" {
		Log.SetOutput(io.Discard)
		return
	}
	Log.SetReportCaller(true)
	Log.SetOutput(os.Stdout)
	Log.SetLevel(logrus.InfoLevel)
}
