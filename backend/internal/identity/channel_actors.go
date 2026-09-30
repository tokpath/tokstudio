package identity

import "context"

// ChannelActors is metadata for scoped audit display, without credentials or
// user profile fields. The service still validates every requested channel.
func (s *Service) ChannelActors(ctx context.Context, viewer Principal, channelIDs []string) (map[string]string, error) {
	actors := make(map[string]string)
	if len(channelIDs) == 0 {
		return actors, nil
	}
	for _, channelID := range channelIDs {
		if _, err := s.GetChannel(ctx, viewer, channelID); err != nil {
			return nil, err
		}
	}
	var rows []userRow
	if err := s.db.WithContext(ctx).Select("id, email").Where("channel_org_id IN ?", channelIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		actors[row.ID] = row.Email
	}
	return actors, nil
}
