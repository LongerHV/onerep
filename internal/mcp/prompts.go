package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const reviewBlock = `Review my latest completed training block in onerep and propose the next one.
1. Call get_plan (the plan I follow) and list_sessions for the block's weeks.
2. For the main lifts, call get_exercise_stats; look at e1RM trend, rep maxes, and how actual sets compared with what was prescribed (get_session).
3. Call get_weekly_muscle_volume for the block to check volume per muscle.
4. Summarise what went well, what stalled, and why, then propose changes (loads, volume, exercise swaps).
5. Write the next block with get_plan_schema's format, check it with validate_plan, and save it with save_plan_draft (plan_id of my plan). Update training maxes with set_training_max only if I agree.
6. Give me the review_url so I can compare and activate it.`

const buildPlan = `Help me build a new training plan in onerep.
1. Ask about my goals, how many days a week I train and for how long, my experience, injuries, and my equipment.
2. Look at my recent training (list_sessions, get_exercise_stats for main lifts) and weekly volume (get_weekly_muscle_volume).
3. Read get_plan_schema, and pick exercises with list_exercises (create_exercise only if nothing fits).
4. Draft the plan, check it with validate_plan, fix every error, and save it with save_plan_draft.
5. If loads use pct_tm, suggest training maxes and set them with set_training_max once I agree.
6. Give me the review_url so I can review and activate the plan.`

func (s *Server) addPrompts(srv *sdk.Server) {
	add := func(name, description, text string) {
		srv.AddPrompt(&sdk.Prompt{Name: name, Description: description},
			func(context.Context, *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
				return &sdk.GetPromptResult{Description: description, Messages: []*sdk.PromptMessage{
					{Role: "user", Content: &sdk.TextContent{Text: text}}}}, nil
			})
	}
	add("review_block", "Analyse the latest completed block and propose the next as a draft.", reviewBlock)
	add("build_plan", "Gather goals, schedule and equipment, then draft a plan.", buildPlan)
}
