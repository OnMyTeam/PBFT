//go run parsing.go [읽을 파일] [내보낼 파일이름] [추출하고자 하는 단어]
package main
 
import (
    "os"
    "fmt"
    "bufio"
    "strings"
)
 
func main() {
    // 입력파일 열기
    fi, err := os.Open(os.Args[1])
    if err != nil {
        panic(err)
    }
    defer fi.Close()
 
    // 출력파일 생성
    fo, err := os.Create(os.Args[2])
    if err != nil {
        panic(err)
    }
    defer fo.Close()

	    scanner:=bufio.NewScanner(fi)
	    for scanner.Scan(){
		line:= scanner.Text()
		line_split:=strings.Split(line,"[")
		fmt.Println(len(line_split))
		if(len(line_split)>1){
			line_split=strings.Split(line_split[1],"]")
			keyword:=line_split[0]
			fmt.Println(keyword)
			if(keyword==os.Args[3]){
				_,err:=fo.Write([]byte(line+"\n"))
				if err!=nil{
					panic("파일 쓰기 실패")
				}
			}
		}
	    }
 
}
