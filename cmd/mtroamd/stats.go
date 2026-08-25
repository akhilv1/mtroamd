package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AG-Studio-Apps/mtroamd/internal/telemetry"
)

func statsCmd(args []string) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output one sample as JSON")
	stream := fs.Bool("stream", false, "stream NDJSON forever")
	interval := fs.Duration("interval", 5*time.Second, "stream interval")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Cancel on stdin EOF
	go func() {
		var buf [1]byte
		for {
			_, err := os.Stdin.Read(buf[:])
			if err != nil {
				cancel()
				return
			}
		}
	}()

	sampler := telemetry.NewSampler()
	writer := bufio.NewWriter(os.Stdout)

	collectAndPrint := func() error {
		sample, err := sampler.Collect(ctx)
		if err != nil {
			return fmt.Errorf("collect: %w", err)
		}
		if *jsonOut || *stream {
			line, err := json.Marshal(sample)
			if err != nil {
				return fmt.Errorf("marshal: %w", err)
			}
			if _, err := writer.Write(append(line, '\n')); err != nil {
				return err
			}
			if err := writer.Flush(); err != nil {
				return err
			}
		} else {
			printHuman(writer, sample)
			if err := writer.Flush(); err != nil {
				return err
			}
		}
		return nil
	}

	if *stream {
		for {
			if err := collectAndPrint(); err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(*interval):
			}
		}
	}

	// One-shot: prime the CPU counters first. Percent is a delta between two
	// ticks, so a single cold sample would always report 0 — which is exactly
	// what `mtroamd stats --json | jq .cpu_pct` is meant to show.
	if _, err := sampler.Collect(ctx); err != nil {
		return fmt.Errorf("prime sampler: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(200 * time.Millisecond):
	}

	if err := collectAndPrint(); err != nil {
		return err
	}
	return nil
}

func printHuman(w io.Writer, s telemetry.Sample) {
	fmt.Fprintf(w, "host: %s\n", s.Host)
	fmt.Fprintf(w, "uptime: %ds\n", s.UptimeSec)
	fmt.Fprintf(w, "load: %.2f %.2f %.2f\n", s.Load[0], s.Load[1], s.Load[2])
	fmt.Fprintf(w, "cpu: %.2f%%\n", s.CPUPct)
	fmt.Fprintf(w, "mem: %d/%d used\n", s.Mem.Used, s.Mem.Total)
	fmt.Fprintf(w, "swap: %d/%d used\n", s.Swap.Used, s.Swap.Total)
	fmt.Fprintf(w, "disks: %d\n", len(s.Disks))
	fmt.Fprintf(w, "net: %d interfaces\n", len(s.Net))
	fmt.Fprintf(w, "temps: %d\n", len(s.Temps))
	fmt.Fprintf(w, "sessions: total=%d attached=%d agent=%d\n", s.Sessions.Total, s.Sessions.Attached, s.Sessions.Agent)
}
