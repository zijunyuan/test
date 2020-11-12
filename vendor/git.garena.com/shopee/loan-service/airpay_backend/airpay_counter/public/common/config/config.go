// Package config toml语法配置组件, ../conf/config.toml这个配置文件只支持 [section] key=value 格式， 如果需要更加复杂的toml语法配置，可以自己再新建一个配置文件，然后调用这里的parse接口。
package config

import (
	"flag"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/mcuadros/go-defaults"
	"io/ioutil"
	"time"
)

var (
	ConfPath = "conf/conf.toml" //可通过命令行参数修改 -conf path
	ConfBuf  []byte
	AppName  string //prometheus会用到
)

// Parse parse config with default and config file ../conf/config.toml
func Parse(c interface{}) error {
	defaults.SetDefaults(c)
	return ParseWithoutDefaults(c)
}

// ParseWithPath 自己定义配置文件路径
func ParseWithPath(c interface{}, path string) error {
	//defaults.SetDefaults(c)
	fmt.Printf("\nconfig file:%s\n", path)
	if _, err := toml.DecodeFile(path, c); err != nil {
		fmt.Println(err)
		return err
	}
	fmt.Printf("toml config: %+v\n", c)
	return nil
}

func ParseWithoutDefaults(c interface{}) error {
	if len(ConfBuf) == 0 {
		fmt.Printf("\nconfig file:%s\n", ConfPath)
		var err error
		ConfBuf, err = ioutil.ReadFile(ConfPath)
		if err != nil {
			fmt.Println(err)
			return err
		}
	}
	_, err := toml.Decode(string(ConfBuf), c)
	if err != nil {
		fmt.Println(err)
		return err
	}
	fmt.Printf("toml config: %+v\n", c)
	return nil
}

// Duration duration for config parse
type Duration time.Duration

func (d Duration) String() string {
	dd := time.Duration(d)
	return dd.String()
}

// GoString  duration go string
func (d Duration) GoString() string {
	dd := time.Duration(d)
	return dd.String()
}

// Duration duration
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// Value duration
func (d Duration) Value() time.Duration {
	return time.Duration(d)
}

// UnmarshalText 字符串解析时间
func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	dd, err := time.ParseDuration(string(text))
	if err == nil {
		*d = Duration(dd)
	}
	return err
}

func init() {
	flag.StringVar(&ConfPath, "conf", "conf/conf.toml", "-conf path")
	//ConfPath = *flag.String("conf","conf/config.toml","-conf path")
	flag.Parse()

	tmp := struct {
		AppName string
	}{}
	ParseWithoutDefaults(&tmp)
	AppName = tmp.AppName
}
