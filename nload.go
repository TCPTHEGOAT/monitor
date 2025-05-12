package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"github.com/eiannone/keyboard"
        "net"
)

const (
	GRAPH_HEIGHT = 20
	GRAPH_WIDTH  = 140
	WINDOW_TIME  = 60.0

	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
	colorBlue   = "\033[34m"
)

type CPUStats struct {
	id      int
	usage   float64
	temp    float64
	freq    float64
	prev    cpuTime
	current cpuTime
}
type cpuTime struct {
	user    uint64
	nice    uint64
	system  uint64
	idle    uint64
	iowait  uint64
	irq     uint64
	softirq uint64
	steal   uint64
}
type LoadAvg struct {
	one     float64
	five    float64
	fifteen float64
}
type Traffic struct {
	current    float64
	average    float64
	min        float64
	max        float64
	total      uint64
	values     []float64
	timestamps []time.Time
}
type NetworkStats struct {
	incoming Traffic
	outgoing Traffic
}

func getCPUModel() string {
	data, err := ioutil.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "Unknown CPU"
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "model name") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "Unknown CPU"
}
func getCPUTemp(id int) float64 {
	sources := []string{
		fmt.Sprintf("/sys/class/hwmon/hwmon*/temp%d_input", id+1),
		"/sys/class/thermal/thermal_zone*/temp",
		fmt.Sprintf("/sys/devices/platform/coretemp.0/hwmon/hwmon*/temp%d_input", id+1),
	}
	for _, pattern := range sources {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			continue
		}
		for _, path := range matches {
			data, err := ioutil.ReadFile(path)
			if err != nil {
				continue
			}
			temp, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
			if err != nil {
				continue
			}
			return temp / 1000.0
		}
	}

	return 0.0
}
func getLoadAvg() LoadAvg {
	data, err := ioutil.ReadFile("/proc/loadavg")
	if err != nil {
		return LoadAvg{}
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return LoadAvg{}
	}
	one, _ := strconv.ParseFloat(fields[0], 64)
	five, _ := strconv.ParseFloat(fields[1], 64)
	fifteen, _ := strconv.ParseFloat(fields[2], 64)
	return LoadAvg{
		one:     one,
		five:    five,
		fifteen: fifteen,
	}
}

func getCPUFreq(id int) float64 {
	sources := []string{
		fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cpufreq/scaling_cur_freq", id),
		fmt.Sprintf("/proc/cpuinfo"),
	}
	data, err := ioutil.ReadFile(sources[0])
	if err == nil {
		freq, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err == nil {
			return freq / 1000000.0
		}
	}
	data, err = ioutil.ReadFile(sources[1])
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "cpu MHz") {
				fields := strings.Split(line, ":")
				if len(fields) == 2 {
					freq, err := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
					if err == nil {
						return freq / 1000.0
					}
				}
			}
		}
	}

	return 0.0
}
func getCPUStats() ([]CPUStats, error) {
	data, err := ioutil.ReadFile("/proc/stat")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	stats := make([]CPUStats, 0)
	for _, line := range lines {
		if strings.HasPrefix(line, "cpu") && len(line) > 3 && line[3] >= '0' && line[3] <= '9' {
			fields := strings.Fields(line)
			if len(fields) < 8 {
				continue
			}
			cpuID, _ := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu"))
			var t cpuTime
			t.user, _ = strconv.ParseUint(fields[1], 10, 64)
			t.nice, _ = strconv.ParseUint(fields[2], 10, 64)
			t.system, _ = strconv.ParseUint(fields[3], 10, 64)
			t.idle, _ = strconv.ParseUint(fields[4], 10, 64)
			t.iowait, _ = strconv.ParseUint(fields[5], 10, 64)
			t.irq, _ = strconv.ParseUint(fields[6], 10, 64)
			t.softirq, _ = strconv.ParseUint(fields[7], 10, 64)
			if len(fields) > 8 {
				t.steal, _ = strconv.ParseUint(fields[8], 10, 64)
			}
			temp := getCPUTemp(cpuID)

			stats = append(stats, CPUStats{
				id:      cpuID,
				current: t,
				temp:    temp,
			})
		}
	}

	return stats, nil
}

func calculateCPUUsage(prev, curr cpuTime) float64 {
	prevIdle := prev.idle + prev.iowait
	currIdle := curr.idle + curr.iowait
	prevNonIdle := prev.user + prev.nice + prev.system + prev.irq + prev.softirq + prev.steal
	currNonIdle := curr.user + curr.nice + curr.system + curr.irq + curr.softirq + curr.steal

	prevTotal := prevIdle + prevNonIdle
	currTotal := currIdle + currNonIdle
	totalDiff := currTotal - prevTotal
	idleDiff := currIdle - prevIdle
	if totalDiff == 0 {
		return 0
	}

	return (1.0 - float64(idleDiff)/float64(totalDiff)) * 100
}
func drawCPUInfo(cpuStats []CPUStats, loadAvg LoadAvg) string {
	var result strings.Builder
	barWidth := 8
	currentFreq := getCPUFreq(0)
	result.WriteString("┌─┐")
	result.WriteString(fmt.Sprintf("%sEPYC 4244P%s", colorBlue, colorReset))
	result.WriteString("")
	result.WriteString("┌")
	result.WriteString(strings.Repeat("─", 7))
	result.WriteString(fmt.Sprintf("%s%.1f GHz%s", colorBlue, currentFreq, colorReset))
	result.WriteString("┌┐\n")
	var totalUsage float64
	var totalTemp float64
	for _, cpu := range cpuStats {
		totalUsage += cpu.usage
		totalTemp += cpu.temp
	}
	avgUsage := totalUsage / float64(len(cpuStats))
	avgTemp := totalTemp / float64(len(cpuStats))
	result.WriteString(fmt.Sprintf("│CPU  %s %3d%% %s %2.0f°│\n",
		getUsageBlocks(avgUsage),
		int(avgUsage),
		getUsageBar(avgUsage, barWidth),
		avgTemp))
	for i := 0; i < 5; i++ {
		if i < len(cpuStats) {
			result.WriteString(fmt.Sprintf("│C%-2d  %s %3d%% %s %2.0f°│\n",
				i,
				getUsageBlocks(cpuStats[i].usage),
				int(cpuStats[i].usage),
				getUsageBar(cpuStats[i].usage, barWidth),
				cpuStats[i].temp))
		}
	}
	result.WriteString(fmt.Sprintf("│Load AVG: %5.2f %5.2f %5.2f │\n",
		loadAvg.one, loadAvg.five, loadAvg.fifteen))
	result.WriteString("└" + strings.Repeat("─", 28) + "┘")

	return result.String()
}

func getUsageBlocks(usage float64) string {
	var result strings.Builder
	blocks := 5
	filledBlocks := int((usage / 100.0) * float64(blocks))
	var color string
	switch {
	case usage >= 70:
		color = colorRed
	case usage >= 40:
		color = colorYellow
	default:
		color = colorGreen
	}
	result.WriteString(color)
	result.WriteString(strings.Repeat("█", filledBlocks))
	result.WriteString(colorReset)
	if filledBlocks < blocks {
		result.WriteString("\033[2m")
		result.WriteString(strings.Repeat("█", blocks-filledBlocks))
		result.WriteString(colorReset)
	}

	return result.String()
}

func getUsageBar(usage float64, width int) string {
	var bar strings.Builder
	filledChars := int((usage / 100.0) * float64(width))
	var color string
	switch {
	case usage >= 70:
		color = colorRed
	case usage >= 40:
		color = colorYellow
	default:
		color = colorGreen
	}
	for i := 0; i < width; i++ {
		if i < filledChars {
			bar.WriteString(color + "⣿" + colorReset)
		} else {
			bar.WriteString("⣀")
		}
	}

	return bar.String()
}

func cleanup() {
	fmt.Print("\033[?25h")
	fmt.Print("\033[2J")
	fmt.Print("\033[H")
	keyboard.Close()
	os.Exit(0)
}
func getInterfaceIP(interfaceName string) string {
    iface, err := net.InterfaceByName(interfaceName)
    if err != nil {
        return ""
    }

    addrs, err := iface.Addrs()
    if err != nil {
        return ""
    }

    for _, addr := range addrs {
        if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
            if ipv4 := ipnet.IP.To4(); ipv4 != nil {
                return ipv4.String()
            }
        }
    }
    return ""
}

func newTraffic() Traffic {
	return Traffic{
		min:        math.MaxFloat64,
		values:     make([]float64, 0, GRAPH_WIDTH),
		timestamps: make([]time.Time, 0, GRAPH_WIDTH),
	}
}
func formatBytesWithPadding(bits float64) string {
	if bits < 0 {
		bits = 0
	}
	units := []string{"kbit/s", "Mbit/s", "Gbit/s", "Tbit/s"}
	unitIndex := 0
	value := bits / 1000.0

	for value >= 1000.0 && unitIndex < len(units)-1 {
		value /= 1000.0
		unitIndex++
	}
	return fmt.Sprintf("%6.2f %s", value, units[unitIndex])
}

func formatTotalBytes(bytes float64) string {
	units := []string{"Byte", "KByte", "MByte", "GByte", "TByte"}
	unitIndex := 0
	value := bytes
	for value >= 1000.0 && unitIndex < len(units)-1 {
		value /= 1000.0
		unitIndex++
	}

	return fmt.Sprintf("%.2f %s", value, units[unitIndex])
}
func formatStats(traffic *Traffic) string {
	return fmt.Sprintf("Curr: %s%s%s\nAvg:  %s%s%s\nMin:  %s%s%s\nMax:  %s%s%s\nTtl: %s",
		colorReset, formatBytesWithPadding(traffic.current), colorReset,
		colorReset, formatBytesWithPadding(traffic.average), colorReset,
		colorReset, formatBytesWithPadding(traffic.min), colorReset,
		colorReset, formatBytesWithPadding(traffic.max), colorReset,
		formatTotalBytes(float64(traffic.total)))
}

func drawGraph(traffic *Traffic, maxValue float64) string {
	var result strings.Builder
	if maxValue == 0 {
		maxValue = 1
	}
	maxValue = maxValue * 1.2
	scaleValues := make([]float64, GRAPH_HEIGHT)
	for i := 0; i < GRAPH_HEIGHT; i++ {
		factor := float64(GRAPH_HEIGHT-i) / float64(GRAPH_HEIGHT)
		scaleValues[i] = maxValue * math.Pow(factor, 1.5)
	}

	statsLines := strings.Split(formatStats(traffic), "\n")
	for i := 0; i < GRAPH_HEIGHT; i++ {
		threshold := scaleValues[i]
		for _, value := range traffic.values {
			if value >= threshold {
				if value >= threshold*1.5 {
					result.WriteString(colorGreen + "#" + colorReset)
				} else {
					result.WriteString("#")
				}
			} else {
				if i == GRAPH_HEIGHT-1 {
					result.WriteString(".")
				} else {
					result.WriteString(" ")
				}
			}
		}

		if i < len(statsLines) {
			result.WriteString("    " + statsLines[i])
		}
		result.WriteString("\n")
	}

	return result.String()
}

func getNetworkStats(interfaceName string) (uint64, uint64, error) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, interfaceName) {
			fields := strings.Fields(line)
			rxBytes, _ := strconv.ParseUint(fields[1], 10, 64)
			txBytes, _ := strconv.ParseUint(fields[9], 10, 64)
			return rxBytes, txBytes, nil
		}
	}
	return 0, 0, fmt.Errorf("interface %s not found", interfaceName)
}
func updateTraffic(t *Traffic, newBytes uint64, prevBytes uint64, now time.Time, interval float64) {
	rate := float64(newBytes-prevBytes) * 8.0 / interval
	t.total = newBytes
	t.current = rate

	if rate < t.min || t.min == math.MaxFloat64 {
		t.min = rate
	}
	if rate > t.max {
		t.max = rate
	}

	t.values = append(t.values, rate)
	t.timestamps = append(t.timestamps, now)
	if len(t.values) > GRAPH_WIDTH {
		t.values = t.values[1:]
		t.timestamps = t.timestamps[1:]
	}
	var sum float64
	count := 0
	windowStart := now.Add(-time.Duration(WINDOW_TIME) * time.Second)
	for i, ts := range t.timestamps {
		if ts.After(windowStart) {
			sum += t.values[i]
			count++
		}
	}
	if count > 0 {
		t.average = sum / float64(count)
	}
}
func listInterfaces() []string {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil
	}
	defer file.Close()
	var interfaces []string
	scanner := bufio.NewScanner(file)
	scanner.Scan()
	scanner.Scan()
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, ":"); idx != -1 {
			iface := strings.TrimSpace(line[:idx])
			if iface != "lo" {
				interfaces = append(interfaces, iface)
			}
		}
	}
	return interfaces
}
func handleKeyboard(interfaces []string, currentIdx *int, done chan bool) {
	if err := keyboard.Open(); err != nil {
		panic(err)
	}
	for {
		char, key, err := keyboard.GetKey()
		if err != nil {
			continue
		}
		switch {
		case key == keyboard.KeyArrowRight:
			*currentIdx = (*currentIdx + 1) % len(interfaces)
		case key == keyboard.KeyArrowLeft:
			*currentIdx = (*currentIdx - 1)
			if *currentIdx < 0 {
				*currentIdx = len(interfaces) - 1
			}
		case char == 'q' || char == 'Q':
			done <- true
			return
		}
	}
}

func main() {
	updateMs := flag.Int("t", 200, "update interval in milliseconds (1-500)")
	flag.Parse()
	if *updateMs < 1 || *updateMs > 500 {
		fmt.Println("Error: interval must be between 1 & 500 milliseconds")
		fmt.Println("\nusage examples:")
		fmt.Println("  ./nload -t 100    # update every 100ms")
		fmt.Println("  ./nload           # default (200ms)")
		os.Exit(1)
	}
	interfaces := listInterfaces()
	if len(interfaces) == 0 {
		fmt.Println("No network interfaces found")
		os.Exit(1)
	}
	currentIdx := 0
	done := make(chan bool)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cleanup()
	}()
	stats := make(map[string]*NetworkStats)
	prevBytes := make(map[string]struct{ rx, tx uint64 })
	for _, iface := range interfaces {
		stats[iface] = &NetworkStats{
			incoming: newTraffic(),
			outgoing: newTraffic(),
		}
		rx, tx, _ := getNetworkStats(iface)
		prevBytes[iface] = struct{ rx, tx uint64 }{rx, tx}
	}
	cpuStats, _ := getCPUStats()
	go handleKeyboard(interfaces, &currentIdx, done)
	updateInterval := time.Duration(*updateMs) * time.Millisecond
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()
	fmt.Print("\033[?25l")
	defer cleanup()

	for {
		select {
		case <-done:
			cleanup()
			return
		case now := <-ticker.C:
			interfaceName := interfaces[currentIdx]
			currentStats := stats[interfaceName]
			rxBytes, txBytes, err := getNetworkStats(interfaceName)
			if err != nil {
				continue
			}
			newCPUStats, _ := getCPUStats()
			for i := range newCPUStats {
				newCPUStats[i].usage = calculateCPUUsage(
					cpuStats[i].current,
					newCPUStats[i].current,
				)

			}
			cpuStats = newCPUStats
			loadAvg := getLoadAvg()
			prev := prevBytes[interfaceName]
			intervalSeconds := float64(*updateMs) / 1000.0

			updateTraffic(&currentStats.incoming, rxBytes, prev.rx, now, intervalSeconds)
			updateTraffic(&currentStats.outgoing, txBytes, prev.tx, now, intervalSeconds)

			inMax := math.Max(currentStats.incoming.max, currentStats.incoming.current*1.2)
			outMax := math.Max(currentStats.outgoing.max, currentStats.outgoing.current*1.2)

			fmt.Print("\033[H\033[2J\033[3J")
fmt.Printf("Device %s%s%s [%s] (%d/%d):\n",
    colorCyan, interfaceName, colorReset, getInterfaceIP(interfaceName),
    currentIdx+1, len(interfaces))
			fmt.Println(strings.Repeat("=", 120))
			fmt.Printf("%sIncoming:%s\n", colorGreen, colorReset)
			fmt.Print(drawGraph(&currentStats.incoming, inMax))
			fmt.Printf("\n%sOutgoing:%s\n", colorGreen, colorReset)
			fmt.Print(drawGraph(&currentStats.outgoing, outMax))
			fmt.Printf("\nuse left/right arrow keys to switch interfaces, use 'q' to quit")
			fmt.Printf("\n\n%s", drawCPUInfo(cpuStats, loadAvg))

			prevBytes[interfaceName] = struct{ rx, tx uint64 }{rxBytes, txBytes}
		}
	}
}
