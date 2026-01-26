package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

const (
	// Alpine image for root operations
	alpineImage = "alpine:latest"
	// Timeout for container operations
	rootOpTimeout = 30 * time.Second
)

// RemovePathAsRoot removes a path using Docker with root privileges.
// This is useful for removing directories that were created by containers
// running as root, which cannot be removed by non-root users.
func RemovePathAsRoot(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Ensure path is absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer cli.Close()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, rootOpTimeout)
	defer cancel()

	// Ensure alpine image is available
	if err := ensureImage(ctx, cli, alpineImage); err != nil {
		return fmt.Errorf("failed to ensure alpine image: %w", err)
	}

	// Create container to remove the path contents
	// Note: We use "sh -c" to delete contents because /target is a mount point
	// and cannot be removed directly. After contents are removed, we remove
	// the empty directory from the host.
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: alpineImage,
		Cmd:   []string{"sh", "-c", "rm -rf /target/* /target/.*  2>/dev/null; exit 0"},
	}, &container.HostConfig{
		Binds:      []string{absPath + ":/target"},
		AutoRemove: true,
	}, nil, nil, "")
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		// Try to remove container if start failed
		_ = cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{Force: true})
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for container to finish
	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("error waiting for container: %w", err)
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return fmt.Errorf("container exited with non-zero status: %d", status.StatusCode)
		}
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for container: %w", ctx.Err())
	}

	// Remove the now-empty directory from host
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove empty directory: %w", err)
	}

	return nil
}

// ensureImage pulls the image if it doesn't exist locally
func ensureImage(ctx context.Context, cli *client.Client, imageName string) error {
	// Check if image exists
	_, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err == nil {
		return nil // Image exists
	}

	// Pull image
	reader, err := cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	// Consume the reader to complete the pull
	_, err = io.Copy(io.Discard, reader)
	return err
}
