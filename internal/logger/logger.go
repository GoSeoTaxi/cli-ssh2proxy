package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var L *zap.Logger

func Init(debug bool) {
	encCfg := zapcore.EncoderConfig{
		TimeKey:      "ts",
		LevelKey:     "lvl",
		MessageKey:   "msg",
		CallerKey:    "caller",
		EncodeTime:   zapcore.ISO8601TimeEncoder,
		EncodeLevel:  zapcore.LowercaseLevelEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
	}
	level := zap.InfoLevel
	if debug {
		level = zap.DebugLevel
	}
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encCfg),
		zapcore.AddSync(os.Stdout),
		level,
	)
	L = zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(L)
	zap.RedirectStdLog(L)
}
