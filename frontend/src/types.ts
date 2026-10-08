export type AppMode = 'preview' | 'demo' | 'connected'

export type PipelineForm = {
  serviceName: string
  repoUrl: string
  projectDescription: string
  branch: string
  imageName: string
  mode: AppMode
  harnessAccountId: string
  hasHarnessToken: boolean
  orgIdentifier: string
  projectIdentifier: string
  codebaseConnectorRef: string
  dockerConnectorRef: string
  includeDeploy: boolean
  k8sConnectorRef: string
  namespace: string
}

export type PipelineStep = {
  id: string
  label: string
  kind: string
  description: string
  command?: string
}

export type SetupItem = {
  id: string
  label: string
  description: string
  requiredFor: string
  status: 'missing' | 'optional' | 'next'
}

export type PipelineResponse = {
  pipelineIdentifier: string
  yaml: string
  nodes: PipelineStep[]
  analysis: string[]
  recommendations: string[]
  missingSetup: SetupItem[]
  mode: AppMode
  notes: string[]
}

export type ProjectAnalysisResponse = {
  defaults: PipelineForm
  analysis: string[]
  recommendations: string[]
  missingSetup: SetupItem[]
  mode: AppMode
  signals: RepoSignals
  analysisSource: 'model' | 'rules'
}

export type RepoSignals = {
  owner: string
  name: string
  branch: string
  language: 'go' | 'node'
  workdir: string
  frameworks: string[]
  hasDockerfile: boolean
  hasTests: boolean
  hasLockfile: boolean
  hasKubernetes: boolean
  files: string[]
}
export type SessionInfo = { authenticated?: boolean; email?: string; csrfToken?: string; freshUntil?: string }
export type HarnessResource = { identifier: string; name: string; type: string }
export type RunRecord = {
  id: string
  mode: AppMode
  accountId: string
  orgIdentifier: string
  projectIdentifier: string
  pipelineIdentifier: string
  executionId: string
  repoUrl: string
  createdAt: string
}
export type DemoConfig = { configured: boolean; repository: string; pipelineIdentifier: string }

export type HarnessConnection = {
  accountId: string
  apiToken: string
}

export type RequestStatus = 'idle' | 'loading' | 'success' | 'error'

export type RoutePath = '/' | '/preview' | '/demo' | '/connected'
