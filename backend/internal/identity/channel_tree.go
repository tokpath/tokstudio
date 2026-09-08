package identity

import "strings"

const (
	ChannelTypeA = "A"
	ChannelTypeB = "B"
	ChannelTypeC = "C"
)

// ValidateChannelParent 渠道树：A 建 B/C；B 无下属；C 只建 B。空 parent 视为挂在官方 A 下。
func ValidateChannelParent(parentType, childType string) error {
	childType = strings.TrimSpace(childType)
	if childType != ChannelTypeB && childType != ChannelTypeC {
		return ErrPromotionInvalid
	}
	if parentType == "" {
		parentType = ChannelTypeA
	}
	switch parentType {
	case ChannelTypeA:
		return nil
	case ChannelTypeB:
		return ErrChannelImmutable
	case ChannelTypeC:
		if childType != ChannelTypeB {
			return ErrChannelImmutable
		}
		return nil
	default:
		return ErrPromotionInvalid
	}
}

// PoolChannelID 积分池所属渠道：C 下的 B 跟 C。
func PoolChannelID(channelType, channelID string, parentID, parentType string) string {
	if channelType == ChannelTypeB && parentType == ChannelTypeC && parentID != "" {
		return parentID
	}
	return channelID
}
