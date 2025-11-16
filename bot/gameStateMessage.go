package bot

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/automuteus/automuteus/v8/internal/server"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
)

// bumped for public rollout. Don't need to update the status message more than once every 2 secs prob
const DeferredEditSeconds = 2
const colorSelectID = "select-color"

type GameStateMessage struct {
	MessageID        string `json:"messageID"`
	MessageChannelID string `json:"messageChannelID"`
	LeaderID         string `json:"leaderID"`
	CreationTimeUnix int64  `json:"creationTimeUnix"`
}

func MakeGameStateMessage() GameStateMessage {
	return GameStateMessage{
		MessageID:        "",
		MessageChannelID: "",
		LeaderID:         "",
		CreationTimeUnix: 0,
	}
}

func (gsm *GameStateMessage) Exists() bool {
	return gsm.MessageID != "" && gsm.MessageChannelID != ""
}

func (dgs *GameState) DeleteGameStateMsg(s *discordgo.Session, reset bool) bool {
	retValue := false
	if dgs.GameStateMsg.Exists() {
		err := s.ChannelMessageDelete(dgs.GameStateMsg.MessageChannelID, dgs.GameStateMsg.MessageID)
		if err != nil {
			retValue = false
		} else {
			retValue = true
		}
	}
	// whether or not we were successful in deleting the message, reset the state
	if reset {
		dgs.GameStateMsg = MakeGameStateMessage()
	}
	return retValue
}

var DeferredEdits = make(map[string]*discordgo.MessageEmbed)
var DeferredEditsLock = sync.Mutex{}

// ==== 色情報マスタ ====
//  key: 英語の色名キーワード（label や value に含まれる文字）
type colorInfo struct {
	LabelJP string // ラベル（絵文字＋カタカナ）
	Emoji   string // ボタンの Emoji.Name として送る Unicode 絵文字
}

var colorInfoMap = []struct {
	Key string
	Info colorInfo
}{
	{"red", colorInfo{LabelJP: "🟥 レッド", Emoji: "🟥"}},
	{"black", colorInfo{LabelJP: "⬛ ブラック", Emoji: "⬛"}},
	{"white", colorInfo{LabelJP: "⬜ ホワイト", Emoji: "⬜"}},
	{"rose", colorInfo{LabelJP: "🌸 ローズ", Emoji: "🌸"}},

	{"blue", colorInfo{LabelJP: "🔵 ブルー", Emoji: "🔵"}},
	{"cyan", colorInfo{LabelJP: "🟦 シアン", Emoji: "🟦"}},
	{"yellow", colorInfo{LabelJP: "🟨 イエロー", Emoji: "🟨"}},
	{"pink", colorInfo{LabelJP: "💗 ピンク", Emoji: "💗"}},

	{"purple", colorInfo{LabelJP: "🟣 パープル", Emoji: "🟣"}},
	{"orange", colorInfo{LabelJP: "🟧 オレンジ", Emoji: "🟧"}},
	{"banana", colorInfo{LabelJP: "🍌 バナナ", Emoji: "🍌"}},
	{"coral", colorInfo{LabelJP: "🧱 コーラル", Emoji: "🧱"}},

	{"lime", colorInfo{LabelJP: "🥬 ライム", Emoji: "🥬"}},
	{"green", colorInfo{LabelJP: "🌲 グリーン", Emoji: "🌲"}},
	{"gray", colorInfo{LabelJP: "⬜ グレー", Emoji: "⬜"}},
	{"maroon", colorInfo{LabelJP: "🍷 マルーン", Emoji: "🍷"}},

	{"brown", colorInfo{LabelJP: "🤎 ブラウン", Emoji: "🤎"}},
	{"tan", colorInfo{LabelJP: "🟫 タン", Emoji: "🟫"}},
}

// 色ボタン用のラベル＆絵文字決定
func buildColorButtonMeta(opt discordgo.SelectMenuOption) (label string, emojiName string) {
	// ✖ はずす（X）用
	if opt.Value == X || strings.EqualFold(opt.Label, X) {
		return "✖ はずす", "✖"
	}

	// label と value をまとめて小文字に
	lower := strings.ToLower(opt.Label + " " + opt.Value)

	// 色名キーワードにマッチしたら、その情報を使う
	for _, entry := range colorInfoMap {
		if strings.Contains(lower, entry.Key) {
			return entry.Info.LabelJP, entry.Info.Emoji
		}
	}

	// どれにもマッチしなかった場合のフォールバック
	// → 絵文字はとりあえず白四角、ラベルは元のラベルのまま
	return opt.Label, "⬜"
}

// Note this is not a pointer; we never expect the underlying DGS to change on an edit
func (dgs GameState) dispatchEdit(s *discordgo.Session, me *discordgo.MessageEmbed) (newEdit bool) {
	if !ValidFields(me) {
		return false
	}

	DeferredEditsLock.Lock()

	// if it isn't found, then start the worker to wait to start it (this is a UNIQUE edit)
	if _, ok := DeferredEdits[dgs.GameStateMsg.MessageID]; !ok {
		go deferredEditWorker(s, dgs.GameStateMsg.MessageChannelID, dgs.GameStateMsg.MessageID)
		newEdit = true
	}
	// whether or not it's found, replace the contents with the new message
	DeferredEdits[dgs.GameStateMsg.MessageID] = me
	DeferredEditsLock.Unlock()
	return newEdit
}

func (dgs GameState) shouldRefresh() bool {
	// discord dictates that we can't edit messages that are older than 1 hour
	return (time.Now().Sub(time.Unix(dgs.GameStateMsg.CreationTimeUnix, 0))) > time.Hour
}

func ValidFields(me *discordgo.MessageEmbed) bool {
	for _, v := range me.Fields {
		if v == nil {
			return false
		}
		if v.Name == "" || v.Value == "" {
			return false
		}
	}
	return true
}

func RemovePendingDGSEdit(messageID string) {
	DeferredEditsLock.Lock()
	delete(DeferredEdits, messageID)
	DeferredEditsLock.Unlock()
}

func deferredEditWorker(s *discordgo.Session, channelID, messageID string) {
	time.Sleep(time.Second * time.Duration(DeferredEditSeconds))

	DeferredEditsLock.Lock()
	me := DeferredEdits[messageID]
	delete(DeferredEdits, messageID)
	DeferredEditsLock.Unlock()

	if me != nil {
		editMessageEmbed(s, channelID, messageID, me)
	}
}

// ===== ここからボタン式 色選択付きの CreateMessage =====

func (dgs *GameState) CreateMessage(s *discordgo.Session, me *discordgo.MessageEmbed, channelID string, authorID string) bool {
	// 元々のセレクトメニュー用オプションを流用
	opts := EmojisToSelectMenuOptions(GlobalAlivenessEmojis[true], X)

	const maxPerRow = 5
	var components []discordgo.MessageComponent
	curRow := discordgo.ActionsRow{}

	for idx, opt := range opts {
		customID := fmt.Sprintf("%s:%s", colorSelectID, opt.Value)

		label, emojiName := buildColorButtonMeta(opt)

		btn := discordgo.Button{
			CustomID: customID,
			Label:    label,
			Style:    discordgo.SecondaryButton,
			Emoji: discordgo.ComponentEmoji{
				Name: emojiName, // ← Unicode 絵文字だけを使用
			},
		}

		curRow.Components = append(curRow.Components, btn)

		// 5 個ごとに改行
		if (idx+1)%maxPerRow == 0 {
			components = append(components, curRow)
			curRow = discordgo.ActionsRow{}
		}
	}

	// 余りがあれば最後の行として追加
	if len(curRow.Components) > 0 {
		components = append(components, curRow)
	}

	msg := sendEmbedWithComponents(s, channelID, me, components)
	if msg != nil {
		dgs.GameStateMsg.LeaderID = authorID
		dgs.GameStateMsg.MessageChannelID = msg.ChannelID
		dgs.GameStateMsg.MessageID = msg.ID
		dgs.GameStateMsg.CreationTimeUnix = time.Now().Unix()
		return true
	}
	return false
}

// ===== ここまで CreateMessage =====

func (bot *Bot) DispatchRefreshOrEdit(readOnlyDgs *GameState, dgsRequest GameStateRequest, sett *settings.GuildSettings) {
	if readOnlyDgs.shouldRefresh() {
		bot.RefreshGameStateMessage(dgsRequest, sett)
	} else {
		edited := readOnlyDgs.dispatchEdit(bot.PrimarySession, bot.gameStateResponse(readOnlyDgs, sett))
		if edited {
			server.RecordDiscordRequests(bot.RedisInterface.client, server.MessageEdit, 1)
		}
	}
}
