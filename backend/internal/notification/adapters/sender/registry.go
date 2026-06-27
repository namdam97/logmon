package sender

import (
	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// Registry trả map channel type → Sender mặc định cho mọi loại kênh hỗ trợ.
// Worker tra theo msg.ChannelType.
func Registry() map[string]ports.Sender {
	return map[string]ports.Sender{
		domain.ChannelSlack.String():     NewSlackSender(),
		domain.ChannelTeams.String():     NewTeamsSender(),
		domain.ChannelWebhook.String():   NewWebhookSender(),
		domain.ChannelPagerDuty.String(): NewPagerDutySender(),
		domain.ChannelEmail.String():     NewEmailSender(),
	}
}
