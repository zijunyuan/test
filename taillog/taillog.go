package taillog

import (
	"fmt"
	"github.com/hpcloud/tail"
)

var (
	tails *tail.Tail
)

//专门从日志文件收集日志的模块

func Init(fileName string) (err error) {
	config := tail.Config{
		ReOpen:    true,                                 //重新打开
		Follow:    true,                                 //是否跟随
		Location:  &tail.SeekInfo{Offset: 0, Whence: 2}, //从文件的哪个地方开始读
		MustExist: false,                                //文件不存在报错
		Poll:      true,
	}
	tails, err = tail.TailFile(fileName, config) //打开文件
	if err != nil {
		fmt.Println("tail file failed,err", err)
		return
	}
	return
}

func ReadChan() <-chan *tail.Line {
	return tails.Lines
}
