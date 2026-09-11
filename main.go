package main

import (
	"disk/internal/router"
	"disk/pkg/setting"
	"log"
)

func main() {
	config, err := setting.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	InitRouter(config)
}

func InitRouter(config *setting.Config) {
	r, err := router.NewRouter()
	if err != nil {
		log.Fatal(err)
	}

	post := config.Server.Listen + ":" + config.Server.Port
	r.Run(post)
}

//func printenv() {
//	for _, env := range os.Environ() {
//		fmt.Println(env)
//	}
//}
