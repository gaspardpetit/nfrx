package openai

import (
    "encoding/json"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
)

// embeddingUsage and embeddingResponse are defined in embeddings.go; reuse types.

type embeddingPartitionJob struct {
    base map[string]json.RawMessage
    inputs []json.RawMessage
    final []json.RawMessage
    usage embeddingUsage
    model string
}

func newEmbeddingPartitionJob(payload map[string]json.RawMessage, inputs []json.RawMessage) *embeddingPartitionJob {
    base := make(map[string]json.RawMessage, len(payload))
    for k, v := range payload { if k != "input" { base[k] = v } }
    return &embeddingPartitionJob{ base: base, inputs: inputs, final: make([]json.RawMessage, len(inputs)) }
}

func (j *embeddingPartitionJob) Size() int { return len(j.inputs) }

func (j *embeddingPartitionJob) MakeChunk(start, count int) ([]byte, int) {
    if start >= len(j.inputs) { return nil, 0 }
    end := start + count
    if end > len(j.inputs) { end = len(j.inputs) }
    b, _ := json.Marshal(j.inputs[start:end])
    mp := make(map[string]json.RawMessage, len(j.base)+1)
    for k, v := range j.base { mp[k] = v }
    mp["input"] = b
    body, _ := json.Marshal(mp)
    return body, end - start
}

func (j *embeddingPartitionJob) Append(resp []byte, start int) error {
    var r embeddingResponse
    _ = json.Unmarshal(resp, &r)
    copy(j.final[start:], r.Data)
    j.usage.PromptTokens += r.Usage.PromptTokens
    j.usage.TotalTokens += r.Usage.TotalTokens
    if j.model == "" { j.model = r.Model }
    return nil
}

func (j *embeddingPartitionJob) Result() []byte {
    out := embeddingResponse{ Object: "list", Data: j.final, Model: j.model, Usage: j.usage }
    b, _ := json.Marshal(out)
    return b
}

func (j *embeddingPartitionJob) Path() string { return "/embeddings" }

func (j *embeddingPartitionJob) DesiredChunkSize(w spi.WorkerRef) int {
    // Use worker's preferred size; no override at job level for now.
    return w.PreferredBatchSize()
}
