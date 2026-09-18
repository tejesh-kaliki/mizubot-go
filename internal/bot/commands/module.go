/* Package commands
*
* This includes all the discord command registrations, with each file registering one module
 */
package commands

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// guildModPermissions are the permissions we treat as "runs this server",
// used to gate anything that edits server-wide bot configuration (the system
// prompt, content flags, etc).
var guildModPermissions = []struct {
	bit  int64
	name string
}{
	{discordgo.PermissionAdministrator, "Administrator"},
	{discordgo.PermissionManageServer, "Manage Server"},
	{discordgo.PermissionManageMessages, "Manage Messages"},
	{discordgo.PermissionModerateMembers, "Moderate Members"},
	{discordgo.PermissionKickMembers, "Kick Members"},
	{discordgo.PermissionBanMembers, "Ban Members"},
}

// canEditGuildConfig reports whether the interaction's caller may edit this
// server's bot configuration: either they are the bot owner (who may edit
// any server regardless of their permissions there), or Discord's computed
// per-channel permissions for them include one of guildModPermissions.
func canEditGuildConfig(i *discordgo.InteractionCreate, ownerDiscordID string) bool {
	if ownerDiscordID != "" && userIDFromInteraction(i) == ownerDiscordID {
		return true
	}
	if i.Member == nil {
		return false
	}
	for _, perm := range guildModPermissions {
		if i.Member.Permissions&perm.bit != 0 {
			return true
		}
	}
	return false
}

func guildModPermissionNames() string {
	names := make([]string, 0, len(guildModPermissions))
	for _, perm := range guildModPermissions {
		names = append(names, perm.name)
	}
	return strings.Join(names, ", ")
}

type Responder interface {
	Respond(i *discordgo.InteractionCreate, content string, ephemeral bool)
	RespondEmbed(i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, ephemeral bool)
	// RespondEmbedWithComponents is RespondEmbed plus message components (buttons,
	// select menus), for a fresh reply such as opening an interactive panel.
	RespondEmbedWithComponents(i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, ephemeral bool)
	RespondModal(i *discordgo.InteractionCreate, customID, title string, components []discordgo.MessageComponent) error
	// UpdateMessage edits the message a component interaction (button click,
	// select menu) was attached to, in place.
	UpdateMessage(i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error
}

type Module interface {
	Definitions() []*discordgo.ApplicationCommand
	Handle(responder Responder, s *discordgo.Session, i *discordgo.InteractionCreate) bool
}
