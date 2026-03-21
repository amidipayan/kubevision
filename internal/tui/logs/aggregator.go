package logs

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/charmbracelet/lipgloss"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)


func StreamMultiPodLogs(ctx context.Context, client kubernetes.Interface, pods []corev1.Pod) (io.ReadCloser, error) {
	reader, writer := io.Pipe()

	var wg sync.WaitGroup

	
	colors := []lipgloss.Color{
		lipgloss.Color("#00FF00"), 
		lipgloss.Color("#00FFFF"), 
		lipgloss.Color("#FF00FF"), 
		lipgloss.Color("#FFFF00"), 
		lipgloss.Color("#FF5555"), 
		lipgloss.Color("#5555FF"), 
		lipgloss.Color("#FFA500"), 
	}

	for i, pod := range pods {
		
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		wg.Add(1)
		color := colors[i%len(colors)]
		
		go func(p corev1.Pod, c lipgloss.Color) {
			defer wg.Done()

			
			prefix := lipgloss.NewStyle().Foreground(c).Bold(true).Render(fmt.Sprintf("[%s]", p.Name))

			opts := &corev1.PodLogOptions{
				Follow:     true,
				Timestamps: true, 
				TailLines:  int64Ptr(300),
			}

			req := client.CoreV1().Pods(p.Namespace).GetLogs(p.Name, opts)
			stream, err := req.Stream(ctx)
			if err != nil {
				fmt.Fprintf(writer, "%s Error connecting: %v\n", prefix, err)
				return
			}
			defer stream.Close()

			scanner := bufio.NewScanner(stream)
			for scanner.Scan() {
				
				select {
				case <-ctx.Done():
					return
				default:
					
					fmt.Fprintf(writer, "%s %s\n", prefix, scanner.Text())
				}
			}
		}(pod, color)
	}

	
	go func() {
		wg.Wait()
		writer.Close()
	}()

	return reader, nil
}

func int64Ptr(i int64) *int64 { return &i }