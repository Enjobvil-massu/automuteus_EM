package command

import (
	"fmt"
	"strings" // ホストURL整形用

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

	// ★ デフォルトは「実行者だけに見える」エフェメラル
	flags := discordgo.MessageFlagsEphemeral

	switch status {
	case NewSuccess:
		// ===== /start 成功時 =====
		content = ""

		// ---- ホストの見た目を整える ----
		host := info.MinimalURL

		// ① :443 を消す（https のデフォルトポートなので見た目だけ削る）
		host = strings.TrimSuffix(host, ":443")

		// ② wss にしたくなった場合（今は使わない）:
		// host = strings.Replace(host, "https://", "wss://", 1)

		// コードの下に出したい注意文
		note := "接続後、AmongUsCapture がフリーズする場合があります。\nその場合はキャプチャを再起動し、再度【登録】ボタンを押してください。"

		embeds = []*discordgo.MessageEmbed{
			{
				Title: "【AmongUsCapture と接続してください】",
				Description: fmt.Sprintf(
					"AmongUsCapture の🔌設定画面で下記を入力してください。\n\n",
				),
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:   "ホスト",
						Value:  fmt.Sprintf("```%s```", host),
						Inline: false,
					},
					{
						Name: "コード",
						// コードのすぐ下に注意文を表示
						Value: fmt.Sprintf("```%s```\n%s", info.ConnectCode, note),
						Inline: false,
					},
				},
			},
		}

	case NewNoVoiceChannel:
		// ボイスチャンネル未参加 → エフェメラルのまま（自分だけにエラー）
		content = sett.LocalizeMessage(&i18n.Message{
			ID:    "commands.new.nochannel",
			Other: "Please join a voice channel before starting a match!",
		})

	case NewLockout:
		// ロックアウト警告はみんなに見えて欲しいので「公開メッセージ」に切り替え
		content = sett.LocalizeMessage(&i18n.Message{
			ID: "commands.new.lockout",
			Other: "If I start any more games, Discord will lock me out, or throttle the games I'm running! 😦\n" +
				"Please try again in a few minutes, or consider AutoMuteUs Premium (`/premium`)\n" +
				"Current Games: {{.Games}}",
		}, map[string]interface{}{
			"Games": fmt.Sprintf("%d/%d", info.ActiveGames, DefaultMaxActiveGames),
		})

		// ここだけ Flags を 0 にして公開メッセージに
		flags = discordgo.MessageFlags(0)
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
