package dto

import "encoding/json"

type PutDataRequest struct {
	Data json.RawMessage `json:"data"`
}

type GetDataResponse struct {
	Data json.RawMessage `json:"data"`
}
