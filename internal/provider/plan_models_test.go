package provider

import (
	"fmt"
)

const zcodeBuiltinJSON = `{"revision":%d,"config":{
 "providerConfigRules":{"providerRules":[
  {"providerId":"account:zai-individual-coding-plan","builtinModelIds":["GLM-5.3"]},
  {"providerId":"account:zai-start-plan","builtinModelIds":["GLM-5-Turbo"]}]},
 "modelConfigRules":{
  "modelRules":[
   {"modelMatch":".*","config":{"properties":{"contextWindow":200000},"optionSpecs":{"maxOutputTokens":{"max":32000},"reasoningLevel":{"values":["disabled","enabled"]}}}},
   {"modelMatch":".*glm-5\\.3(?:-flash)?(?:[.\\-:/\\[].*)?","config":{"properties":{"contextWindow":1000000},"optionSpecs":{"maxOutputTokens":{"max":128000},"reasoningLevel":{"values":["low","high","max"]}}}},
   {"modelMatch":".*glm-5\\.3-flash(?:[.\\-:/\\[].*)?","config":{"properties":{"inputFormat":{"supportsImage":true}}}}],
  "builtinProviderModelRules":[
   {"providerId":"account:zai-individual-coding-plan","modelId":"GLM-5.3-Flash","config":{"enabled":true}},
   {"providerId":"account:zai-individual-coding-plan","modelId":"GLM-5.1","config":{"enabled":false}},
   {"providerId":"account:zai-individual-coding-plan","modelId":"GLM-5-Turbo","config":{"enabled":true}},
   {"providerId":"account:bigmodel-individual-coding-plan","modelId":"GLM-X","config":{"enabled":true}}]}}}`

func zcodeBuiltinAt(rev int) string { return fmt.Sprintf(zcodeBuiltinJSON, rev) }
