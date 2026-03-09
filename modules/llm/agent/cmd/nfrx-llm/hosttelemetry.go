package main

import (
	"errors"
	"fmt"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"

	wp "github.com/gaspardpetit/nfrx/sdk/base/agent/workerproxy"
)

func buildHostTelemetry() (map[string]string, func() (wp.HeartbeatSample, error), error) {
	info, err := host.Info()
	if err != nil {
		return nil, nil, err
	}
	if _, err := cpu.Percent(0, false); err != nil {
		return nil, nil, fmt.Errorf("prime cpu counters: %w", err)
	}
	agentConfig := map[string]string{
		"host_os":               info.OS,
		"host_platform":         info.Platform,
		"host_platform_family":  info.PlatformFamily,
		"host_platform_version": info.PlatformVersion,
		"host_kernel_version":   info.KernelVersion,
		"host_hostname":         info.Hostname,
	}
	sampler := func() (wp.HeartbeatSample, error) {
		sample := wp.HeartbeatSample{}
		var errs []error
		cpuPercents, err := cpu.Percent(0, false)
		if err != nil {
			errs = append(errs, fmt.Errorf("sample cpu percent: %w", err))
		} else if len(cpuPercents) == 0 {
			errs = append(errs, fmt.Errorf("sample cpu percent: empty result"))
		} else {
			sample.HostCPUPercent = cpuPercents[0]
		}
		vm, err := mem.VirtualMemory()
		if err != nil {
			errs = append(errs, fmt.Errorf("sample memory usage: %w", err))
		} else {
			sample.HostRAMUsedPercent = vm.UsedPercent
		}
		return sample, errors.Join(errs...)
	}
	return agentConfig, sampler, nil
}
