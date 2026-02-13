//go:build linux

package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func findOpenCodeServerPIDImpl(_ string, port string) (int, error) {
	port = strings.TrimSpace(port)
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", port, err)
	}
	inodes, err := listeningSocketInodesByPort(portNum)
	if err != nil {
		return 0, err
	}
	if len(inodes) == 0 {
		return 0, nil
	}
	pids, err := pidsBySocketInodes(inodes)
	if err != nil {
		return 0, err
	}
	if len(pids) == 0 {
		return 0, nil
	}
	sort.Ints(pids)
	return pids[0], nil
}

func readOpenCodeProcessCmdlineImpl(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("invalid pid: %d", pid)
	}
	path := fmt.Sprintf("/proc/%d/cmdline", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}
	text := strings.ReplaceAll(string(data), "\x00", " ")
	return strings.TrimSpace(text), nil
}

func listeningSocketInodesByPort(port int) (map[string]struct{}, error) {
	files := []string{"/proc/net/tcp", "/proc/net/tcp6"}
	inodes := map[string]struct{}{}
	for _, file := range files {
		lines, err := readProcNetTCPLines(file)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			state := strings.TrimSpace(fields[3])
			if state != "0A" {
				continue
			}
			local := strings.TrimSpace(fields[1])
			addrParts := strings.Split(local, ":")
			if len(addrParts) != 2 {
				continue
			}
			value, err := strconv.ParseInt(addrParts[1], 16, 32)
			if err != nil {
				continue
			}
			if int(value) != port {
				continue
			}
			inode := strings.TrimSpace(fields[9])
			if inode != "" {
				inodes[inode] = struct{}{}
			}
		}
	}
	return inodes, nil
}

func readProcNetTCPLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return []string{}, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= 1 {
		return []string{}, nil
	}
	return lines[1:], nil
}

func pidsBySocketInodes(inodes map[string]struct{}) ([]int, error) {
	rootEntries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	seen := map[int]struct{}{}
	for _, entry := range rootEntries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		fdEntries, err := os.ReadDir(filepath.Join("/proc", entry.Name(), "fd"))
		if err != nil {
			continue
		}
		for _, fd := range fdEntries {
			target, err := os.Readlink(filepath.Join("/proc", entry.Name(), "fd", fd.Name()))
			if err != nil {
				continue
			}
			inode := parseSocketInode(target)
			if inode == "" {
				continue
			}
			if _, ok := inodes[inode]; ok {
				seen[pid] = struct{}{}
				break
			}
		}
	}
	out := make([]int, 0, len(seen))
	for pid := range seen {
		out = append(out, pid)
	}
	return out, nil
}

func parseSocketInode(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "socket:[") || !strings.HasSuffix(value, "]") {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(value, "socket:["), "]")
}
