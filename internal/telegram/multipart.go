package telegram

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"strings"
)

// createMultipartAudio 构建 multipart/form-data 用于上传音频
func createMultipartAudio(file *os.File, filename string, chatID int64, title, artist string, duration int) (io.Reader, string, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer writer.Close()

		_ = writer.WriteField("chat_id", fmt.Sprintf("%d", chatID))
		_ = writer.WriteField("title", title)
		_ = writer.WriteField("performer", artist)
		_ = writer.WriteField("duration", fmt.Sprintf("%d", duration))
		_ = writer.WriteField("parse_mode", "HTML")

		part, err := writer.CreateFormFile("audio", filename)
		if err != nil {
			return
		}
		io.Copy(part, file)
	}()

	return pr, writer.FormDataContentType(), nil
}

// FormatCaption 格式化消息文本
func FormatCaption(title, artist, album, quality string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎵 <b>%s</b>\n", title))
	sb.WriteString(fmt.Sprintf("👤 %s\n", artist))
	if album != "" {
		sb.WriteString(fmt.Sprintf("💿 %s\n", album))
	}
	sb.WriteString(fmt.Sprintf("📀 %s", quality))
	return sb.String()
}
