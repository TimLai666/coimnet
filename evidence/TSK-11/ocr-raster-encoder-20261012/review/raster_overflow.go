package main
import (
 "context"
 "fmt"
 "image"
 "image/png"
 "os"
 "runtime/debug"
 "github.com/TimLai666/coimnet/tasks/ocr"
)
func main() {
 f,err:=os.OpenFile("runs/.tmp/62/review/one.png",os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600); if err!=nil{panic(err)}
 if err=png.Encode(f,image.NewGray(image.Rect(0,0,1,1)));err!=nil{panic(err)};if err=f.Close();err!=nil{panic(err)}
 defer func(){if r:=recover();r!=nil{fmt.Printf("PANIC: %v\n%s",r,debug.Stack())}}()
 result,err:=ocr.LoadImage(context.Background(),"runs/.tmp/62/review/one.png",ocr.ImageOptions{Height:1<<31,MaxPixels:int(^uint(0)>>1)})
 fmt.Printf("result=%+v error=%v\n",result,err)
}
