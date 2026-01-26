package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func TestRemovePathAsRoot(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "rootfs_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Logf("Created temp dir: %s", tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatal("test file should exist")
	}

	// Remove using RemovePathAsRoot
	if err := RemovePathAsRoot(ctx, tmpDir); err != nil {
		t.Fatalf("RemovePathAsRoot failed: %v", err)
	}

	// Verify directory is removed
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatal("directory should be removed")
	}
}

// TestRemovePathAsRootDebug is a debug test to see what's happening
func TestRemovePathAsRootDebug(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "rootfs_debug")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir) // cleanup if test fails

	t.Logf("Temp dir: %s", tmpDir)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	// Get absolute path
	absPath, _ := filepath.Abs(tmpDir)
	t.Logf("Absolute path: %s", absPath)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer cli.Close()

	// Create container with ls command to debug
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"ls", "-la", "/target"},
	}, &container.HostConfig{
		Binds: []string{absPath + ":/target"},
	}, nil, nil, "")
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	// Wait for container
	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		t.Logf("Wait error: %v", err)
	case status := <-statusCh:
		t.Logf("Exit status: %d", status.StatusCode)
	}

	// Get logs
	logs, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		t.Logf("failed to get logs: %v", err)
	} else {
		defer logs.Close()
		buf := make([]byte, 4096)
		n, _ := logs.Read(buf)
		t.Logf("Container logs:\n%s", string(buf[:n]))
	}

	// Cleanup
	cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{Force: true})
}

func TestRemovePathAsRoot_EmptyPath(t *testing.T) {
	ctx := context.Background()
	err := RemovePathAsRoot(ctx, "")
	if err == nil {
		t.Fatal("should return error for empty path")
	}
}
