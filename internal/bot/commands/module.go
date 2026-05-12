/* Package commands
*
* This includes all the discord command registrations, with each file registering one module
 */
package commands

import "github.com/bwmarrin/discordgo"

type Responder interface {
	Respond(i *discordgo.InteractionCreate, content string, ephemeral bool)
	RespondEmbed(i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, ephemeral bool)
}

type Module interface {
	Definitions() []*discordgo.ApplicationCommand
	Handle(responder Responder, s *discordgo.Session, i *discordgo.InteractionCreate) bool
}
