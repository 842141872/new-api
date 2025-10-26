package helper

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// extractContextSizeFromModel 从模型名称中提取上下文大小（单位：k）
// 示例：gpt-4-100k -> 100, claude-[200K] -> 200, model(128k) -> 128
// 如果模型名有多个匹配，返回最大值
func extractContextSizeFromModel(modelName string) int {
	if modelName == "" {
		return 0
	}

	// 正则：匹配数字+k/K
	re := regexp.MustCompile(`(\d+)[kK]`)
	matches := re.FindAllStringSubmatch(modelName, -1)

	if len(matches) == 0 {
		return 0
	}

	// 如果有多个匹配，取最大值
	maxSize := 0
	for _, match := range matches {
		if len(match) > 1 {
			size, err := strconv.Atoi(match[1])
			if err == nil && size > maxSize {
				maxSize = size
			}
		}
	}

	return maxSize
}

// ValidateContextLimit 验证输入token数量是否超过模型上下文限制
// 返回错误信息（如果超限）
func ValidateContextLimit(modelName string, inputTokens int) error {
	contextSizeK := extractContextSizeFromModel(modelName)

	// 没有上下文标识，不做限制
	if contextSizeK == 0 {
		return nil
	}

	// 计算限制：K值 * 1000
	limit := contextSizeK * 1000

	// 检查是否超限
	if inputTokens > limit {
		// 计算"万"单位
		limitWan := float64(contextSizeK) / 10.0
		inputWan := float64(inputTokens) / 10000.0

		// 格式化"万"的显示
		formatWan := func(wan float64) string {
			if wan == float64(int(wan)) {
				return fmt.Sprintf("%.0f万", wan)
			}
			str := fmt.Sprintf("%.1f万", wan)
			return strings.TrimSuffix(str, ".0万") + "万"
		}

		return fmt.Errorf("输入%d把（%s），本模型的最大输入是%dk（%s），请换更大的上下文模型呀",
			inputTokens, formatWan(inputWan), contextSizeK, formatWan(limitWan))
	}

	return nil
}
