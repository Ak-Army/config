package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/Ak-Army/cli"
	"github.com/Ak-Army/xlog"

	_ "github.com/Ak-Army/config/cmd/configcrypt/command"
)

func main() {
	l := initLogger()
	ctx := xlog.NewContext(context.Background(), l)

	c := cli.New("configcrypt", "1.1.0")
	cli.RootCommand().Authors = []string{"Ak-Army"}
	c.SetDefault("run")
	c.Run(ctx, os.Args)
	time.Sleep(1 * time.Second)
}

func initLogger() xlog.Logger {
	xlog.SetLogger(xlog.NopLogger)
	multiOutput := xlog.MultiOutput{}
	multiOutput = append(multiOutput, xlog.NewConsoleOutput())
	level := xlog.LevelInfo
	for _, v := range os.Args {
		if v == "-v" {
			level = xlog.LevelDebug
		}
	}
	conf := xlog.Config{
		Level:  level,
		Output: multiOutput,
	}
	log.SetFlags(0)
	l := xlog.New(conf)
	xlog.SetLogger(l)
	log.SetOutput(l)

	return l
}
