package model

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Log struct {
	Id               int    `json:"id" gorm:"index:idx_created_at_id,priority:1"`
	UserId           int    `json:"user_id" gorm:"index"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type             int    `json:"type" gorm:"index:idx_created_at_type"`
	Content          string `json:"content"`
	Username         string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName        string `json:"token_name" gorm:"index;default:''"`
	ModelName        string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota            int    `json:"quota" gorm:"default:0"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	UseTime          int    `json:"use_time" gorm:"default:0"`
	IsStream         bool   `json:"is_stream"`
	ChannelId        int    `json:"channel" gorm:"index"`
	ChannelName      string `json:"channel_name" gorm:"->"`
	TokenId          int    `json:"token_id" gorm:"default:0;index"`
	Group            string `json:"group" gorm:"index"`
	Ip               string `json:"ip" gorm:"index;default:''"`
	Other            string `json:"other"`
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
)

func formatUserLogs(logs []*Log) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// delete admin
			delete(otherMap, "admin_info")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = logs[i].Id % 1024
	}
}

func GetLogByKey(key string) (logs []*Log, err error) {
	if os.Getenv("LOG_SQL_DSN") != "" {
		var tk Token
		if err = DB.Model(&Token{}).Where(logKeyCol+"=?", strings.TrimPrefix(key, "sk-")).First(&tk).Error; err != nil {
			return nil, err
		}
		err = LOG_DB.Model(&Log{}).Where("token_id=?", tk.Id).Find(&logs).Error
	} else {
		err = LOG_DB.Joins("left join tokens on tokens.id = logs.token_id").Where("tokens.key = ?", strings.TrimPrefix(key, "sk-")).Find(&logs).Error
	}
	formatUserLogs(logs)
	return logs, err
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, content))
	username := c.GetString("username")
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}

	// 从 other 中提取 prompt_tokens，用于上下文超限等错误
	promptTokens := 0
	completionTokens := 0
	if other != nil {
		if pt, ok := other["prompt_tokens"].(int); ok {
			promptTokens = pt
			// 对于上下文超限错误，补全字数设置为输入字数，方便用户查看
			if errorType, exists := other["error_type"].(string); exists && errorType == "context_limit_exceeded" {
				completionTokens = pt
			}
		}
	}

	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		Other: otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	otherStr := common.MapToJsonStr(params.Other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		Other: otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(userId, username, params.ModelName, params.Quota, common.GetTimestamp(), params.PromptTokens+params.CompletionTokens)
		})
	}
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channels []int, group string, userId int, emptyResponse string, tokenCount int, usernameFuzzy bool, userIdFuzzy bool) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", "%"+modelName+"%")
	}
	if username != "" {
		if usernameFuzzy {
			// 模糊搜索：只搜索 logs.username，避免 JOIN
			// logs.username 字段有索引，查询速度快
			tx = tx.Where("logs.username LIKE ?", "%"+username+"%")
		} else {
			// 精确搜索：只搜索 logs.username，避免 JOIN
			// 使用 = 可以利用索引，比 LIKE 更快
			tx = tx.Where("logs.username = ?", username)
		}
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if len(channels) > 0 {
		tx = tx.Where("logs.channel_id IN ?", channels)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	if userId != 0 {
		if userIdFuzzy {
			// 模糊搜索：将userId转为字符串，进行LIKE匹配
			tx = tx.Where("CAST(logs.user_id AS CHAR) LIKE ?", "%"+strconv.Itoa(userId)+"%")
		} else {
			// 精确搜索：直接匹配用户ID
			tx = tx.Where("logs.user_id = ?", userId)
		}
	}
	// 空回复筛选逻辑
	if emptyResponse == "empty" {
		// 只筛选成功但返回为空的请求，排除错误日志（type=5）
		tx = tx.Where("(logs.completion_tokens = 0 OR logs.completion_tokens IS NULL) AND logs.type != ?", LogTypeError)
	} else if emptyResponse == "non_empty" {
		tx = tx.Where("(logs.completion_tokens > 0 AND logs.completion_tokens IS NOT NULL)")
	}
	// 输入/输出字数精确匹配（OR关系）
	if tokenCount > 0 {
		tx = tx.Where("logs.prompt_tokens = ? OR logs.completion_tokens = ?", tokenCount, tokenCount)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}

		// 收集重试渠道ID
		if log.Other != "" {
			var otherMap map[string]interface{}
			otherMap, _ = common.StrToMap(log.Other)
			if otherMap != nil && otherMap["admin_info"] != nil {
				if adminInfo, ok := otherMap["admin_info"].(map[string]interface{}); ok {
					if useChannel, ok := adminInfo["use_channel"].([]interface{}); ok {
						for _, channelId := range useChannel {
							var id int
							switch v := channelId.(type) {
							case float64:
								id = int(v)
							case int:
								id = v
							}
							if id != 0 {
								channelIds.Add(id)
							}
						}
					}
				}
			}
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
			return logs, total, err
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]

			// 为重试渠道添加名称映射
			if logs[i].Other != "" {
				var otherMap map[string]interface{}
				otherMap, _ = common.StrToMap(logs[i].Other)
				if otherMap != nil && otherMap["admin_info"] != nil {
					if adminInfo, ok := otherMap["admin_info"].(map[string]interface{}); ok {
						// 优先使用已保存的渠道名称
						if useChannelNames, exists := adminInfo["use_channel_names"].([]interface{}); exists && len(useChannelNames) > 0 {
							// 已有保存的渠道名称，直接使用
							channelNames := make([]string, 0, len(useChannelNames))
							for _, name := range useChannelNames {
								if nameStr, ok := name.(string); ok {
									channelNames = append(channelNames, nameStr)
								}
							}
							adminInfo["use_channel_names"] = channelNames
							logs[i].Other = common.MapToJsonStr(otherMap)
						} else if useChannel, ok := adminInfo["use_channel"].([]interface{}); ok && len(useChannel) > 0 {
							// 没有保存的渠道名称，从数据库查询（兼容旧数据）
							channelNames := make([]string, 0, len(useChannel))
							for _, channelId := range useChannel {
								var id int
								switch v := channelId.(type) {
								case float64:
									id = int(v)
								case int:
									id = v
								}
								if name, exists := channelMap[id]; exists {
									channelNames = append(channelNames, name)
								} else {
									channelNames = append(channelNames, fmt.Sprintf("渠道%d", id))
								}
							}
							adminInfo["use_channel_names"] = channelNames
							logs[i].Other = common.MapToJsonStr(otherMap)
						}
					}
				}
			}
		}
	}

	return logs, total, err
}

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, emptyResponse string, tokenCount int) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("logs.user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("logs.user_id = ? and logs.type = ?", userId, logType)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", "%"+modelName+"%")
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	// 空回复筛选逻辑
	if emptyResponse == "empty" {
		// 只筛选成功但返回为空的请求，排除错误日志（type=5）
		tx = tx.Where("(logs.completion_tokens = 0 OR logs.completion_tokens IS NULL) AND logs.type != ?", LogTypeError)
	} else if emptyResponse == "non_empty" {
		tx = tx.Where("(logs.completion_tokens > 0 AND logs.completion_tokens IS NOT NULL)")
	}
	// 输入/输出字数精确匹配（OR关系）
	if tokenCount > 0 {
		tx = tx.Where("logs.prompt_tokens = ? OR logs.completion_tokens = ?", tokenCount, tokenCount)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	formatUserLogs(logs)
	return logs, total, err
}

func SearchAllLogs(keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("type = ? or content LIKE ?", keyword, keyword+"%").Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	return logs, err
}

func SearchUserLogs(userId int, keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("user_id = ? and type = ?", userId, keyword).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs)
	return logs, err
}

type Stat struct {
	Quota int `json:"quota"`
	Rpm   int `json:"rpm"`
	Tpm   int `json:"tpm"`
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channels []int, group string, userId int, emptyResponse string, tokenCount int, usernameFuzzy bool, userIdFuzzy bool) (stat Stat) {
	tx := LOG_DB.Table("logs").Select("sum(quota) quota")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, sum(prompt_tokens) + sum(completion_tokens) tpm")

	// 用户名筛选（与 GetAllLogs 保持一致，只搜索 logs.username，避免 JOIN）
	if username != "" {
		if usernameFuzzy {
			// 模糊搜索
			tx = tx.Where("logs.username LIKE ?", "%"+username+"%")
			rpmTpmQuery = rpmTpmQuery.Where("logs.username LIKE ?", "%"+username+"%")
		} else {
			// 精确搜索
			tx = tx.Where("logs.username = ?", username)
			rpmTpmQuery = rpmTpmQuery.Where("logs.username = ?", username)
		}
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name like ?", "%"+modelName+"%")
		rpmTpmQuery = rpmTpmQuery.Where("model_name like ?", "%"+modelName+"%")
	}
	if len(channels) > 0 {
		tx = tx.Where("channel_id IN ?", channels)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id IN ?", channels)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
	}
	if userId != 0 {
		if userIdFuzzy {
			// 模糊搜索：将userId转为字符串，进行LIKE匹配
			tx = tx.Where("CAST(logs.user_id AS CHAR) LIKE ?", "%"+strconv.Itoa(userId)+"%")
			rpmTpmQuery = rpmTpmQuery.Where("CAST(logs.user_id AS CHAR) LIKE ?", "%"+strconv.Itoa(userId)+"%")
		} else {
			// 精确搜索：直接匹配用户ID
			tx = tx.Where("logs.user_id = ?", userId)
			rpmTpmQuery = rpmTpmQuery.Where("logs.user_id = ?", userId)
		}
	}
	if emptyResponse == "empty" {
		tx = tx.Where("(completion_tokens = 0 OR completion_tokens IS NULL) AND type != ?", LogTypeError)
		rpmTpmQuery = rpmTpmQuery.Where("(completion_tokens = 0 OR completion_tokens IS NULL) AND type != ?", LogTypeError)
	} else if emptyResponse == "non_empty" {
		tx = tx.Where("(completion_tokens > 0 AND completion_tokens IS NOT NULL)")
		rpmTpmQuery = rpmTpmQuery.Where("(completion_tokens > 0 AND completion_tokens IS NOT NULL)")
	}
	// 输入/输出字数筛选（OR关系）
	if tokenCount > 0 {
		tx = tx.Where("logs.prompt_tokens = ? OR logs.completion_tokens = ?", tokenCount, tokenCount)
		rpmTpmQuery = rpmTpmQuery.Where("logs.prompt_tokens = ? OR logs.completion_tokens = ?", tokenCount, tokenCount)
	}

	// 日志类型筛选
	// Quota 统计：始终只统计消费类型（LogTypeConsume），因为只有消费类型才有 quota
	// RPM/TPM 统计：根据用户选择的 logType 参数筛选
	tx = tx.Where("type = ?", LogTypeConsume)

	if logType != LogTypeUnknown {
		// 用户选择了特定类型，RPM/TPM 只统计该类型
		rpmTpmQuery = rpmTpmQuery.Where("type = ?", logType)
	}
	// 如果 logType=0（全部），RPM/TPM 统计所有类型

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	tx.Scan(&stat)
	rpmTpmQuery.Scan(&stat)

	return stat
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}
