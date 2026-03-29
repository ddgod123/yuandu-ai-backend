package videojobs

import "strings"

type aiGIFDirectorPromptPack struct {
	FixedPromptCore         string
	FixedPromptContractTail string
	FixedPromptVersion      string
	FixedPromptSource       string
	ContractTailVersion     string

	OperatorInstructionRaw        string
	OperatorInstructionRendered   string
	OperatorInstructionSchema     map[string]interface{}
	OperatorInstructionRenderMode string
	OperatorVersion               string
	OperatorSource                string
	OperatorEnabled               bool
}

type aiDirectorBuiltInPromptDefaults struct {
	FixedPromptCore         string
	FixedPromptContractTail string
	FixedPromptVersion      string
	FixedPromptSource       string
	ContractTailVersion     string
}

func resolveAIDirectorBuiltInPromptDefaults(targetFormat string) aiDirectorBuiltInPromptDefaults {
	format := normalizeAIPromptTemplateFormat(targetFormat)
	switch format {
	case "png", "jpg", "webp", "live":
		return aiDirectorBuiltInPromptDefaults{
			FixedPromptCore:         defaultAIImageDirectorFixedCorePrompt,
			FixedPromptContractTail: defaultAIGIFDirectorFixedContractTailPrompt,
			FixedPromptVersion:      "built_in_image_v1",
			FixedPromptSource:       "built_in_default_image",
			ContractTailVersion:     "built_in_contract_tail_v1",
		}
	default:
		return aiDirectorBuiltInPromptDefaults{
			FixedPromptCore:         defaultAIGIFDirectorFixedCorePrompt,
			FixedPromptContractTail: defaultAIGIFDirectorFixedContractTailPrompt,
			FixedPromptVersion:      "built_in_v1",
			FixedPromptSource:       "built_in_default",
			ContractTailVersion:     "built_in_contract_tail_v1",
		}
	}
}

func (p *Processor) resolveAIGIFDirectorPromptPack(
	targetFormat string,
	qualitySettings QualitySettings,
) aiGIFDirectorPromptPack {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	builtIn := resolveAIDirectorBuiltInPromptDefaults(targetFormat)

	pack := aiGIFDirectorPromptPack{
		FixedPromptCore:         builtIn.FixedPromptCore,
		FixedPromptContractTail: builtIn.FixedPromptContractTail,
		FixedPromptVersion:      builtIn.FixedPromptVersion,
		FixedPromptSource:       builtIn.FixedPromptSource,
		ContractTailVersion:     builtIn.ContractTailVersion,

		OperatorInstructionRaw: strings.TrimSpace(qualitySettings.AIDirectorOperatorInstruction),
		OperatorVersion:        strings.TrimSpace(qualitySettings.AIDirectorOperatorInstructionVersion),
		OperatorEnabled:        qualitySettings.AIDirectorOperatorEnabled,
		OperatorSource:         "ops.video_quality_settings",
	}
	if pack.OperatorVersion == "" {
		pack.OperatorVersion = "v1"
	}

	if p != nil {
		if fixedTemplate, templateErr := p.loadAIPromptTemplateWithFallback(targetFormat, "ai1", "fixed"); templateErr == nil {
			if fixedTemplate.Found {
				if strings.TrimSpace(fixedTemplate.Text) != "" && fixedTemplate.Enabled {
					pack.FixedPromptCore = strings.TrimSpace(fixedTemplate.Text)
				}
				if strings.TrimSpace(fixedTemplate.Version) != "" {
					pack.FixedPromptVersion = strings.TrimSpace(fixedTemplate.Version)
				}
				if strings.TrimSpace(fixedTemplate.Source) != "" {
					pack.FixedPromptSource = strings.TrimSpace(fixedTemplate.Source)
				}
			}
		}

		if editableTemplate, templateErr := p.loadAIPromptTemplateWithFallback(targetFormat, "ai1", "editable"); templateErr == nil {
			if editableTemplate.Found {
				pack.OperatorInstructionRaw = strings.TrimSpace(editableTemplate.Text)
				pack.OperatorEnabled = editableTemplate.Enabled
				if strings.TrimSpace(editableTemplate.Version) != "" {
					pack.OperatorVersion = strings.TrimSpace(editableTemplate.Version)
				}
				if strings.TrimSpace(editableTemplate.Source) != "" {
					pack.OperatorSource = strings.TrimSpace(editableTemplate.Source)
				}
			}
		}
	}

	pack.OperatorInstructionRendered, pack.OperatorInstructionSchema, pack.OperatorInstructionRenderMode = renderAIDirectorOperatorInstruction(pack.OperatorInstructionRaw)
	if strings.TrimSpace(pack.OperatorInstructionRendered) == "" {
		pack.OperatorInstructionRendered = pack.OperatorInstructionRaw
	}

	return pack
}
