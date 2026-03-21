package utils

import (
	"fmt"
	"math"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)


func HumanizeDuration(d time.Duration) string {
	if d.Seconds() < 60 {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d.Minutes() < 60 {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d.Hours() < 24 {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}

	days := int(math.Floor(d.Hours() / 24))
	return fmt.Sprintf("%dd", days)
}


func ComputeAge(timestamp *metav1.Time) string {
	if timestamp == nil || timestamp.IsZero() {
		return "N/A"
	}
	return HumanizeDuration(time.Since(timestamp.Time))
}