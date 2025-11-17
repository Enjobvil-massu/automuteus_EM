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

	UserData     UserDataSet `json:"userData"`
	VoiceChannel string      `json:"voiceChannel"`

	GameStateMsg GameStateMessage `json:"gameStateMessage"`

	GameData amongus.GameData `json:"amongUsData"`
}

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
	dgs.VoiceChannel = ""
	dgs.GameStateMsg = MakeGameStateMessage()
	dgs.GameData = amongus.NewGameData()
}

// ===== ここ：Member から「ボイス欄と同じ表示名」を取り出すヘルパー =====
func memberDisplayName(m *discordgo.Member) string {
	if m == nil || m.User == nil {
		return ""
	}

	// 1. サーバーニックネーム
	if m.Nick != "" {
		return m.Nick
	}

	// 2. グローバル表示名（新しい「表示名」）
	if m.User.GlobalName != "" {
		return m.User.GlobalName
	}

	// 3. 旧来のユーザー名（@の後ろ）
	if m.User.Username != "" {
		return m.User.Username
	}

	// 最悪IDだけ
	return m.User.ID
}

// ===== ユーザーキャッシュへの登録 =====
func (dgs *GameState) checkCacheAndAddUser(g *discordgo.Guild, s *discordgo.Session, userID string) (UserData, bool) {
	if g == nil {
		return UserData{}, false
	}

	// まず Guild のメンバーキャッシュから探す
	for _, v := range g.Members {
		if v.User != nil && v.User.ID == userID {
			displayName := memberDisplayName(v)

			// displayName を「Nick」として保存する
			user := MakeUserDataFromDiscordUser(v.User, displayName)
			dgs.UserData[v.User.ID] = user
			return user, true
		}
	}

	// キャッシュにいなければ API から取得
	mem, err := s.GuildMember(g.ID, userID)
	if err != nil {
		log.Println(err)
		return UserData{}, false
	}

	displayName := memberDisplayName(mem)

	user := MakeUserDataFromDiscordUser(mem.User, displayName)
	dgs.UserData[mem.User.ID] = user
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
				// ===== リンク済みプレイヤー =====

				// checkCacheAndAddUser で「表示名」を Nick として保存しているので、
				// Nick を最優先で使う
				discordName := userData.GetNickName()
				if discordName == "" {
					discordName = userData.GetUserName()
				}

				// フィールド名：アモアス名（Discord表示名）
				field.Name = fmt.Sprintf("%s（%s）", player.Name, discordName)

				// 本文：状態：<クルー絵文字> 生存/死亡　色：🟥 レッド
				field.Value = fmt.Sprintf(
					"状態：%s %s　色：%s",
					emoji.FormatForInline(),
					statusText,
					colorLabel,
				)

				linked = true
				break
			}
		}

		if !linked {
			// ===== 未リンクプレイヤー =====
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

	// 1人1ブロックで縦並びにするので、パディングは不要
	return sorted
}
