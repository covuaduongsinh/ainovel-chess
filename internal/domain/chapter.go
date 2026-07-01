package domain

import (
	"fmt"
	"unicode/utf8"
)

// ReviewInterval khoảng cách thẩm định toàn cục (kích hoạt mỗi N chương).
const ReviewInterval = 5

// ShouldReview dựa trên số chương đã hoàn thành để xác định có cần thẩm định toàn cục không (chế độ truyện ngắn/truyện vừa).
func ShouldReview(completedCount int) (bool, string) {
	if completedCount > 0 && completedCount%ReviewInterval == 0 {
		return true, fmt.Sprintf("Đã hoàn thành %d chương, kích hoạt thẩm định toàn cục", completedCount)
	}
	return false, ""
}

// ShouldArcReview trong chế độ truyện dài, xác định có cần thẩm định cấp cung/tập không.
func ShouldArcReview(isArcEnd, isVolumeEnd bool, volume, arc int) (bool, string) {
	if isVolumeEnd {
		return true, fmt.Sprintf("Tập %d cung %d kết thúc (tập kết thúc), kích hoạt thẩm định cấp cung+tập", volume, arc)
	}
	if isArcEnd {
		return true, fmt.Sprintf("Tập %d cung %d kết thúc, kích hoạt thẩm định cấp cung", volume, arc)
	}
	return false, ""
}

// WordCount tính số chữ theo rune.
func WordCount(content string) int {
	return utf8.RuneCountInString(content)
}
