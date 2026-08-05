package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSON 自定义类型，用于在 GORM 中存储 JSON 数据
type JSON json.RawMessage

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "null", nil
	}
	return string(j), nil
}

func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = JSON("null")
		return nil
	}
	switch v := value.(type) {
	case string:
		*j = JSON(v)
	case []byte:
		*j = JSON(v)
	default:
		return fmt.Errorf("JSON.Scan: unsupported type %T", value)
	}
	return nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return []byte(j), nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return fmt.Errorf("JSON.UnmarshalJSON: nil pointer")
	}
	*j = append((*j)[0:0], data...)
	return nil
}

// MarshalTo 将结构体序列化为 JSON 类型
func MarshalTo(v interface{}) (JSON, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return JSON(data), nil
}

// UnmarshalTo 将 JSON 类型反序列化为结构体
func UnmarshalTo(j JSON, v interface{}) error {
	return json.Unmarshal([]byte(j), v)
}
