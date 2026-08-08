// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func collectFailures(t *testing.T, dataDir string) []string {
	t.Helper()
	var failures []string
	fail := func(format string, args ...any) {
		failures = append(failures, fmt.Sprintf(format, args...))
	}
	gpuIDs, gpuArches := validateGPUDB(filepath.Join(dataDir, "gpudb.json"), fail)
	cpuIDs, cpuArches := validateCPUDB(filepath.Join(dataDir, "cpudb.json"), fail)
	validateCoefficients(filepath.Join(dataDir, "coefficients.json"), gpuArches, cpuArches, fail)
	validateRequirements(filepath.Join(dataDir, "requirements"), gpuIDs, cpuIDs, fail)
	return failures
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAcceptsWellFormedData(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gpudb.json"), `{"schema_version":1,"gpus":[
		{"canonical_id":"nvidia-geforce-gtx-1060-6gb","market_name":"x","architecture":"nvidia-pascal","shader_units":1280,"boost_clock_ghz":1.7,"bus_width_bits":192,"effective_mem_clock_ghz":8}
	]}`)
	writeFile(t, filepath.Join(dir, "cpudb.json"), `{"schema_version":1,"cpus":[
		{"canonical_id":"intel-core-i5-8400","market_name":"x","vendor_id":"GenuineIntel","family":6,"model":158,"phys_cores":6,"logical_cores":6,"architecture":"intel-skylake"}
	]}`)
	writeFile(t, filepath.Join(dir, "coefficients.json"), `{"schema_version":1,
		"gpu_arch":{"nvidia-pascal":{"coefficient":1.0}},
		"cpu_ipc":{"intel-skylake":{"coefficient":1.0}}
	}`)

	if got := collectFailures(t, dir); len(got) != 0 {
		t.Errorf("got failures %v, want none", got)
	}
}

func TestValidateCatchesDuplicateCanonicalID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gpudb.json"), `{"schema_version":1,"gpus":[
		{"canonical_id":"dup","architecture":"a","shader_units":1,"boost_clock_ghz":1,"bus_width_bits":1,"effective_mem_clock_ghz":1},
		{"canonical_id":"dup","architecture":"a","shader_units":1,"boost_clock_ghz":1,"bus_width_bits":1,"effective_mem_clock_ghz":1}
	]}`)
	writeFile(t, filepath.Join(dir, "cpudb.json"), `{"schema_version":1,"cpus":[]}`)
	writeFile(t, filepath.Join(dir, "coefficients.json"), `{"schema_version":1,"gpu_arch":{"a":{"coefficient":1}},"cpu_ipc":{}}`)

	failures := collectFailures(t, dir)
	if !containsSubstring(failures, "duplicate canonical_id") {
		t.Errorf("failures = %v, want a duplicate-canonical_id failure", failures)
	}
}

func TestValidateCatchesMissingCoefficientForUsedArchitecture(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gpudb.json"), `{"schema_version":1,"gpus":[
		{"canonical_id":"x","architecture":"nvidia-made-up","shader_units":1,"boost_clock_ghz":1,"bus_width_bits":1,"effective_mem_clock_ghz":1}
	]}`)
	writeFile(t, filepath.Join(dir, "cpudb.json"), `{"schema_version":1,"cpus":[]}`)
	writeFile(t, filepath.Join(dir, "coefficients.json"), `{"schema_version":1,"gpu_arch":{},"cpu_ipc":{}}`)

	failures := collectFailures(t, dir)
	if !containsSubstring(failures, `architecture "nvidia-made-up" with no gpu_arch coefficient`) {
		t.Errorf("failures = %v, want a missing-coefficient failure", failures)
	}
}

// TestValidateCatchesHallucinatedCanonicalID is §12.5's core guard: a
// requirements record naming a canonical ID that doesn't exist in
// gpudb.json/cpudb.json must be rejected — this is exactly the kind of
// mistake an LLM-backed normalizer (§6.4) could make.
func TestValidateCatchesHallucinatedCanonicalID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gpudb.json"), `{"schema_version":1,"gpus":[
		{"canonical_id":"nvidia-geforce-gtx-1060-6gb","architecture":"nvidia-pascal","shader_units":1,"boost_clock_ghz":1,"bus_width_bits":1,"effective_mem_clock_ghz":1}
	]}`)
	writeFile(t, filepath.Join(dir, "cpudb.json"), `{"schema_version":1,"cpus":[]}`)
	writeFile(t, filepath.Join(dir, "coefficients.json"), `{"schema_version":1,"gpu_arch":{"nvidia-pascal":{"coefficient":1}},"cpu_ipc":{}}`)
	writeFile(t, filepath.Join(dir, "requirements", "1.json"), `{
		"appid": 1, "name": "Test", "source": "steam", "review_status": "auto",
		"tiers": {"minimum": {"gpu": {"any_of": ["nvidia-geforce-rtx-3080-super-ti-max"]}}}
	}`)

	failures := collectFailures(t, dir)
	if !containsSubstring(failures, "not a known canonical GPU ID") {
		t.Errorf("failures = %v, want a hallucinated-ID failure", failures)
	}
}

func TestValidateAcceptsRealCanonicalID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gpudb.json"), `{"schema_version":1,"gpus":[
		{"canonical_id":"nvidia-geforce-gtx-1060-6gb","architecture":"nvidia-pascal","shader_units":1,"boost_clock_ghz":1,"bus_width_bits":1,"effective_mem_clock_ghz":1}
	]}`)
	writeFile(t, filepath.Join(dir, "cpudb.json"), `{"schema_version":1,"cpus":[]}`)
	writeFile(t, filepath.Join(dir, "coefficients.json"), `{"schema_version":1,"gpu_arch":{"nvidia-pascal":{"coefficient":1}},"cpu_ipc":{}}`)
	writeFile(t, filepath.Join(dir, "requirements", "1.json"), `{
		"appid": 1, "name": "Test", "source": "steam", "review_status": "auto",
		"tiers": {"minimum": {"gpu": {"any_of": ["nvidia-geforce-gtx-1060-6gb"]}}}
	}`)

	if got := collectFailures(t, dir); len(got) != 0 {
		t.Errorf("got failures %v, want none for a real canonical ID", got)
	}
}

func TestValidateRequiresSourceURLOnHumanVerified(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gpudb.json"), `{"schema_version":1,"gpus":[]}`)
	writeFile(t, filepath.Join(dir, "cpudb.json"), `{"schema_version":1,"cpus":[]}`)
	writeFile(t, filepath.Join(dir, "coefficients.json"), `{"schema_version":1,"gpu_arch":{},"cpu_ipc":{}}`)
	writeFile(t, filepath.Join(dir, "requirements", "1.json"), `{
		"appid": 1, "name": "Test", "source": "steam", "review_status": "human_verified",
		"tiers": {}
	}`)

	failures := collectFailures(t, dir)
	if !containsSubstring(failures, "source_url is empty") {
		t.Errorf("failures = %v, want a missing-source_url failure", failures)
	}
}

func TestValidateMissingRequirementsDirIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gpudb.json"), `{"schema_version":1,"gpus":[]}`)
	writeFile(t, filepath.Join(dir, "cpudb.json"), `{"schema_version":1,"cpus":[]}`)
	writeFile(t, filepath.Join(dir, "coefficients.json"), `{"schema_version":1,"gpu_arch":{},"cpu_ipc":{}}`)
	// no requirements/ directory at all

	if got := collectFailures(t, dir); len(got) != 0 {
		t.Errorf("got failures %v, want none — a missing requirements/ dir is not an error", got)
	}
}

func containsSubstring(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
