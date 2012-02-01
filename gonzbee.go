package main

import (
	"flag"
	"fmt"
	"gonzbee/config"
	"gonzbee/job"
	"gonzbee/nntp"
	"os"
)

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	defer func() {
		if e := recover(); e != nil {
			err := e.(error)
			fmt.Fprintln(os.Stdout, err.Error())
			os.Exit(1)
		}
	}()
	flag.Parse()

	fmt.Println(config.C)
	nzbPath := flag.Arg(0)
	job, err := job.FromFile(nzbPath)
	checkErr(err)

	serverAddress := config.C.Server.GetAddressStr()
	if serverAddress == "" {
		fmt.Fprintf(os.Stdout, "No server address in config")
		os.Exit(1)
	}
	conn, err := nntp.Dial(serverAddress)
	checkErr(err)
	defer conn.Close()

	err = conn.Authenticate(config.C.Server.Username, config.C.Server.Password)
	checkErr(err)
	job.Start(conn)
}
