package bot

import (
	"fmt"
	"log"
	"strings"

	"github.com/automuteus/automuteus/v8/pkg/amongus"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// GameState represents a full record of the entire current game's state. It is intended to be fully JSON-serializable,
// so that any shard/worker can pick up the game state and operate upon it (using locks as necessary)
type GameState struct {
	GuildID string `json:"guildID"`

	ConnectCode string `json:"connectCode"`

	Linked     bool `json:"linked"`
	Running    bool `json:"running"`
	Subscribed bool `json:"subscribed"`

	MatchID        int64 `json:"matchID"`
	MatchStartUnix int64 `json:"matchStartUnix"`

	UserData     UserDataSet       `json:"userData"`
	DisplayNames map[string]string `json:"displayNames"` // 追加: userID -> 表示名（ニックネーム優先）
	VoiceChannel string            `json:"voiceChannel"`
	GameStateMsg GameStateMessage  `json:"gameStateMessage"`
	GameData     amongus.GameData  `json:"amongUsData"`

	// ===== 追加: AmongUsCapture 接続状態 =====
	CaptureConnected bool  `json:"captureConnected"`
	LastCapturePing  int64 `json:"lastCapturePing,omitempty"`
}

// ===== GameState ヘルパー =====

func NewDiscordGameState(guildID string) *GameState {
	dgs := GameState{GuildID: guildID}
	dgs.Reset()
	return &dgs
}

func (dgs *GameState) Reset() {
	// Explicitly does not reset the GuildID!
	dgs.ConnectCode = ""
	dgs.Linked = false
	dgs.Running = false
	dgs.Subscribed = false
	dgs.MatchID = -1
	dgs.MatchStartUnix = -1
	dgs.UserData = map[string]UserData{}
	dgs.DisplayNames = map[string]string{} // 表示名キャッシュもリセット
	dgs.VoiceChannel = ""
	dgs.GameStateMsg = MakeGameStateMessage()
	dgs.GameData = amongus.NewGameData()

	// ===== 追加: Capture未接続で初期化 =====
	dgs.CaptureConnected = false
	dgs.LastCapturePing = 0
}

// ギルドメンバー情報をキャッシュしつつ UserData を作成
func (dgs *GameState) checkCacheAndAddUser(g *discordgo.Guild, s *discordgo.Session, userID string) (UserData, bool) {
	if g == nil {
		return UserData{}, false
	}

	// ===== 1. Guild メンバーキャッシュから探す =====
	for _, m := range g.Members {
		if m.User != nil && m.User.ID == userID {
			user := MakeUserDataFromDiscordUser(m.User, m.Nick)
			dgs.UserData[m.User.ID] = user

			// ニックネーム → なければユーザー名
			display := m.Nick
			if display == "" && m.User != nil {
				display = m.User.Username
			}
			if display != "" {
				if dgs.DisplayNames == nil {
					dgs.DisplayNames = map[string]string{}
				}
				dgs.DisplayNames[m.User.ID] = display
			}

			return user, true
		}
	}

	// ===== 2. API で取得（キャッシュに無い場合） =====
	mem, err := s.GuildMember(g.ID, userID)
	if err != nil {
		log.Println(err)
		return UserData{}, false
	}

	user := MakeUserDataFromDiscordUser(mem.User, mem.Nick)
	dgs.UserData[mem.User.ID] = user

	display := mem.Nick
	if display == "" && mem.User != nil {
		display = mem.User.Username
	}
	if display != "" {
		if dgs.DisplayNames == nil {
			dgs.DisplayNames = map[string]string{}
		}
		dgs.DisplayNames[mem.User.ID] = display
	}

	return user, true
}

//
// ===== ここからプレイヤー表示用の色ラベルヘルパー =====
//

// ボタンと同じ表記用の色マスタ
type colorLabelPattern struct {
	Key   string
	Label string
}

var colorLabelPatterns = []colorLabelPattern{
	{Key: "red", Label: "🟥 レッド"},
	{Key: "black", Label: "⬛ ブラック"},
	{Key: "white", Label: "⬜ ホワイト"},
	{Key: "rose", Label: "🌸 ローズ"},

	{Key: "blue", Label: "🔵 ブルー"},
	{Key: "cyan", Label: "🟦 シアン"},
	{Key: "yellow", Label: "🟨 イエロー"},
	{Key: "pink", Label: "💗 ピンク"},

	{Key: "purple", Label: "🟣 パープル"},
	{Key: "orange", Label: "🟧 オレンジ"},
	{Key: "banana", Label: "🍌 バナナ"},
	{Key: "coral", Label: "🧱 コーラル"},

	{Key: "lime", Label: "🥬 ライム"},
	{Key: "green", Label: "🌲 グリーン"},
	{Key: "gray", Label: "⬜ グレー"},
	{Key: "maroon", Label: "🍷 マルーン"},

	{Key: "brown", Label: "🤎 ブラウン"},
	{Key: "tan", Label: "🟫 タン"},
}

// Emoji 名（例: "AliveRed", "DeadBlue" など）から「🟥 レッド」形式を返す
func colorLabelFromEmojiName(name string) string {
	lower := strings.ToLower(name)
	for _, p := range colorLabelPatterns {
		if strings.Contains(lower, p.Key) {
			return p.Label
		}
	}
	// マッチしなかったときのフォールバック
	return "❓ 不明"
}

//
// ===== ここから Embed のプレイヤー一覧生成 =====
//

// ToEmojiEmbedFields はゲーム状態から Embed のフィールドを生成する
// ・各色ごとに 1 フィールド
// ・フィールド名: アモアス名（ディスコード表示名）
// ・フィールド本文: 状態 と 色の情報
func (dgs *GameState) ToEmojiEmbedFields(emojis AlivenessEmojis, sett *settings.GuildSettings) []*discordgo.MessageEmbedField {
	// 色順で並べるための一時配列（最大 18 色）
	unsorted := make([]*discordgo.MessageEmbedField, 18)
	num := 0

	for _, player := range dgs.GameData.PlayerData {
		if player.Color < 0 || player.Color > 17 {
			break
		}

		// 生存/死亡で別のクルー絵文字を取得
		emoji := emojis[player.IsAlive][player.Color]

		// 状態テキスト（生存 / 死亡）
		statusText := "生存中"
		if !player.IsAlive {
			statusText = "死亡中"
		}

		// ボタンと同じ色表記（🟥 レッド など）
		colorLabel := colorLabelFromEmojiName(emoji.Name)

		field := &discordgo.MessageEmbedField{
			Inline: false, // 1人ずつ改行表示
		}

		linked := false
		for _, userData := range dgs.UserData {
			if userData.InGameName == player.Name {
				// リンク済みプレイヤー

				// userID からキャッシュしておいた表示名を取得
				userID := userData.GetID()
				displayName := ""
				if dgs.DisplayNames != nil {
					displayName = dgs.DisplayNames[userID]
				}
				// キャッシュがない場合のフォールバック（念のため）
				if displayName == "" {
					displayName = userID
				}

				// フィールド名：アモアス名（表示名） ※メンションではないのでピン通知されない
				field.Name = fmt.Sprintf("%s（%s）", player.Name, displayName)

				// 本文：状態：<クルー絵文字> 生存/死亡　色：🟥 レッド
				field.Value = fmt.Sprintf(
					"状態：%s %s　色：%s",
					emoji.FormatForInline(), // クルーの絵文字のみ（🟢 や 💀 は使わない）
					statusText,
					colorLabel,
				)

				linked = true
				break
			}
		}

		if !linked {
			// 未リンクプレイヤー
			unlinkedText := sett.LocalizeMessage(&i18n.Message{
				ID:    "discordGameState.ToEmojiEmbedFields.Unlinked",
				Other: "Unlinked",
			})

			field.Name = fmt.Sprintf("%s（%s）", player.Name, unlinkedText)
			field.Value = fmt.Sprintf(
				"状態：%s %s　色：%s",
				emoji.FormatForInline(),
				statusText,
				colorLabel,
			)
		}

		unsorted[player.Color] = field
		num++
	}

	// 色順に並べ替え
	sorted := make([]*discordgo.MessageEmbedField, 0, num)
	for i := 0; i < 18; i++ {
		if unsorted[i] != nil {
			sorted = append(sorted, unsorted[i])
		}
	}

	// ※1人1ブロックで縦並びにするので、最後の行を埋めるパディングは不要
	return sorted
}
