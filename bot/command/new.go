package command

import (
	"fmt"

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
	// デフォルトは「自分だけ見える」メッセージ
	flags := discordgo.MessageFlagsEphemeral

	switch status {
	case NewSuccess:
		// ===== /start 成功時の見た目 =====
		// AmongUsCapture の Host / Code を日本語で表示
		content = "" // 本文テキストは使わず Embed だけにする

		embeds = []*discordgo.MessageEmbed{
			{
				Title: "🍰 AmongUsCapture を接続してください",
				Description: fmt.Sprintf(
					"AmongUsCapture の設定画面で、下記の値を入力してください。\n\n"+
						"・**Host** → 下の「ホスト」をコピペ\n"+
						"・**Code** → 下の「コード」をコピペ\n\n"+
						"※ キャプチャ本体のダウンロードは <%s> から行えます。",
					CaptureDownloadURL,
				),
				Color: 0x00cc88,
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:  "ホスト",
						Value: fmt.Sprintf("```%s```", info.MinimalURL),
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
		// VC 入ってないときのエラーは既存のまま（必要なら後で日本語化でもOK）
		content = sett.LocalizeMessage(&i18n.Message{
			ID:    "commands.new.nochannel",
			Other: "Please join a voice channel before starting a match!",
		})

	case NewLockout:
		// ロックアウト警告は元のまま（公開メッセージ）
		content = sett.LocalizeMessage(&i18n.Message{
			ID: "commands.new.lockout",
			Other: "If I start any more games, Discord will lock me out, or throttle the games I'm running! 😦\n" +
				"Please try again in a few minutes, or consider AutoMuteUs Premium (`/premium`)\n" +
				"Current Games: {{.Games}}",
		}, map[string]interface{}{
			"Games": fmt.Sprintf("%d/%d", info.ActiveGames, DefaultMaxActiveGames),
		})
		flags = discordgo.MessageFlags(0) // これはみんなに見せる
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
