package transaction_monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

var ipfsGateways = []string{
	"https://ipfs.io/ipfs/",
	"https://gateway.pinata.cloud/ipfs/",
}

const localIpfsUrl = "http://localhost:8080/ipfs/"

type IpfsResult struct {
	CID        string
	RawData    []byte
	SlicedData string
	ParsedData interface{}
	Duration   time.Duration
	Timestamp  string
}

func extractIPFSHash(data string) string {
	re := regexp.MustCompile(`Qm[1-9A-HJ-NP-Za-km-z]{44}`)
	match := re.FindString(data)
	return match
}

func resolveIPFSContent(hash string) ([]byte, error) {
	result, err := getIpfsData(hash)
	if err != nil {
		return nil, err
	}
	return result.RawData, nil
}

func decodeIPFSContent(data []byte) (string, error) {
	var result interface{}
	err := json.Unmarshal(data, &result)
	if err == nil {
		prettyJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to format JSON: %v", err)
		}
		return string(prettyJSON), nil
	}

	// If it's not JSON, return it as a string
	return string(data), nil
}

func getIpfsData(cid string) (*IpfsResult, error) {
	// Try local IPFS node first
	result, err := fetchFromIpfs(localIpfsUrl, cid)
	if err == nil {
		return result, nil
	}

	// If local node fails, try public gateways
	errChan := make(chan error, len(ipfsGateways))
	resultChan := make(chan *IpfsResult, len(ipfsGateways))

	for _, gateway := range ipfsGateways {
		go func(gateway string) {
			result, err := fetchFromIpfs(gateway, cid)
			if err != nil {
				errChan <- err
				return
			}
			resultChan <- result
		}(gateway)
	}

	for i := 0; i < len(ipfsGateways); i++ {
		select {
		case result := <-resultChan:
			return result, nil
		case <-errChan:
			// If we get an error, continue to the next iteration
			continue
		}
	}

	return nil, fmt.Errorf("failed to fetch IPFS data from all sources")
}

func fetchFromIpfs(baseUrl, cid string) (*IpfsResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	startTime := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", baseUrl+cid, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	duration := time.Since(startTime)

	var parsedData interface{}
	err = json.Unmarshal(rawData, &parsedData)
	if err != nil {
		// If it's not valid JSON, treat it as raw data
		parsedData = string(rawData)
	}

	slicedData := string(rawData)
	if len(slicedData) > 100 {
		slicedData = slicedData[:100]
	}

	return &IpfsResult{
		CID:        cid,
		RawData:    rawData,
		SlicedData: slicedData,
		ParsedData: parsedData,
		Duration:   duration,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}
