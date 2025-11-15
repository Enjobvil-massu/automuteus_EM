package command

import (
	"fmt"
	"strings" // ★ 追加

	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

type NewStatus int

const (
	NewSuccess NewStatus = iota
	NewNoVoiceChannel
	NewLockout
)

type NewInfo struct {
	Hyperlink    string
	MinimalURL   string
	ApiHyperlink string
	ConnectCode  string
	ActiveGames  int64
}

// /new → /start にリネーム済み
var New = discordgo.ApplicationCommand{
	Name:        "start",
	Description: "オートミュートを開始します",
}

func NewResponse(status NewStatus, info NewInfo, sett *settings.GuildSettings) *discordgo.InteractionResponse {
	var content string
	var embeds []*discordgo.MessageEmbed
	flags := discordgo.MessageFlagsEphemeral // デフォルトは自分だけ

	switch status {
	case NewSuccess:
		// ===== /start 成功時 =====
		content = ""

		// ---- ホストの見た目を整える ----
		host := info.MinimalURL

		// ① :443 を消す（https のデフォルトポートなので見た目だけ削る）
		host = strings.TrimSuffix(host, ":443")

		// ② もし wss 表記にしたくなったら、これを有効化すればOK
		//    （今は https のまま）
		// host = strings.Replace(host, "https://", "wss://", 1)

		// 追記したい注意メッセージ
		note := "接続後、AmongUsCapture がフリーズする場合があります。\nその場合はキャプチャを再起動し、再度【登録】ボタンを押してください。"

		embeds = []*discordgo.MessageEmbed{
			{
				Title: "🍰 AmongUsCapture を接続してください",
				Description: fmt.Sprintf(
					"AmongUsCapture の🔌設定画面で、下記の値を入力してください。\n\n"+
						"・**Host** → 下の「ホスト」をコピペ\n"+
						"・**Code** → 下の「コード」をコピペ\n\n"+
						"※ キャプチャ本体のダウンロードは <%s> から行えます。\n\n"+
						"%s",
					CaptureDownloadURL,
					note,
				),
				Color: 0x00cc88,
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:   "ホスト",
						Value:  fmt.Sprintf("```%s```", host),
						Inline: false,
					},
					{
						Name:   "コード",
						Value:  fmt.Sprintf("```%s```", info.ConnectCode),
						Inline: true,
					},
				},
			},
		}

	case NewNoVoiceChannel:
		content = sett.LocalizeMessage(&i18n.Message{
			ID:    "commands.new.nochannel",
			Other: "Please join a voice channel before starting a match!",
		})

	case NewLockout:
		content = sett.LocalizeMessage(&i18n.Message{
			ID: "commands.new.lockout",
			Other: "If I start any more games, Discord will lock me out, or throttle the games I'm running! 😦\n" +
				"Please try again in a few minutes, or consider AutoMuteUs Premium (`/premium`)\n" +
				"Current Games: {{.Games}}",
		}, map[string]interface{}{
			"Games": fmt.Sprintf("%d/%d", info.ActiveGames, DefaultMaxActiveGames),
		})
		flags = discordgo.MessageFlags(0) // 公開メッセージ
	}

	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   flags,
			Content: content,
			Embeds:  embeds,
		},
	}
}
