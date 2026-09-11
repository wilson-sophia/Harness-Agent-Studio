import { useEffect, useState, type ReactNode } from 'react'
import { AlertCircle, CheckCircle2, Home, Route, ShieldCheck } from 'lucide-react'
import { analyzeRepository, generatePipeline } from './api'
import './App.css'
import {
  ConnectedPage,
  DemoPage,
  HomePage,
  PreviewPage,
  type PageProps,
} from './pages'
import type {
  AppMode,
  HarnessConnection,
  PipelineForm,
  PipelineStep,
  RequestStatus,
  RoutePath,
  SetupItem,
} from './types'

const defaultForm: PipelineForm = {
  serviceName: 'go-shortlink',
  repoUrl: 'https://github.com/wilson-sophia/go-shortlink',
  projectDescription:
    'Go backend inferred from GitHub repository metadata. The current analyzer assumes Go modules, a Dockerfile-based image build, and CI-first delivery before real deployment.',
  branch: 'main',
  imageName: 'wilson-sophia/go-shortlink',
  mode: 'preview',
  harnessAccountId: '',
  hasHarnessToken: false,
  orgIdentifier: 'default',
  projectIdentifier: 'harness_agent_studio',
  codebaseConnectorRef: 'github_connector',
  dockerConnectorRef: 'dockerhub_connector',
  includeDeploy: false,
  k8sConnectorRef: 'k8s_connector',
  namespace: 'default',
}

const initialSteps: PipelineStep[] = [
  {
    id: 'clone',
    label: 'Clone',
    kind: 'source',
    description: 'Fetch repository code from GitHub.',
  },
  {
    id: 'test',
    label: 'Go Test',
    kind: 'test',
    description: 'Run the Go test suite before building.',
    command: 'go test ./...',
  },
  {
    id: 'build',
    label: 'Go Build',
    kind: 'build',
    description: 'Compile the Go service binary.',
    command: 'go build -o bin/app ./...',
  },
  {
    id: 'docker',
    label: 'Docker Push',
    kind: 'package',
    description: 'Build and push the runtime image.',
  },
]

const initialAnalysis = [
  'Detected repository provider: GitHub.',
  'Repository URL was parsed before pipeline generation.',
  'Inferred service name: go-shortlink.',
  'Inferred Docker image: wilson-sophia/go-shortlink.',
  'Detected Go service workflow from the selected project template.',
]

const initialRecommendations = [
  'Run tests before building the binary to fail fast on code issues.',
  'Use the Harness pipeline sequence id as one Docker tag so every run has a traceable image.',
  'Use the repository Dockerfile for image builds and keep runtime configuration outside the image.',
  'Add deployment as milestone 2 after the CI path is stable.',
]

const initialMissingSetup: SetupItem[] = [
  {
    id: 'harness-account',
    label: 'Harness Account ID',
    description: 'Needed when the user wants this pipeline created in their Harness account.',
    requiredFor: 'Connected Mode',
    status: 'missing',
  },
  {
    id: 'harness-token',
    label: 'Harness API Token',
    description: 'Needed for org, project, connector, pipeline, and execution API calls.',
    requiredFor: 'Connected Mode',
    status: 'missing',
  },
  {
    id: 'git-connector',
    label: 'GitHub Connector',
    description: 'Harness needs a connector that can read the target GitHub repository.',
    requiredFor: 'Pipeline Creation',
    status: 'missing',
  },
  {
    id: 'docker-connector',
    label: 'Docker Registry Connector',
    description: 'Harness needs a connector for pushing the service image.',
    requiredFor: 'Image Publishing',
    status: 'missing',
  },
]

const initialDemoSetup: SetupItem[] = [
  {
    id: 'sandbox-project',
    label: 'Harness Sandbox Project',
    description: 'Platform-owned org, project, and connectors for a trusted demo repository.',
    requiredFor: 'Demo Mode',
    status: 'next',
  },
  {
    id: 'demo-repo-whitelist',
    label: 'Demo Repository Whitelist',
    description: 'Only approved repositories should be executable through the shared sandbox path.',
    requiredFor: 'Safe Demo',
    status: 'next',
  },
  {
    id: 'sandbox-run-view',
    label: 'Run Status View',
    description: 'A later milestone can show Harness execution status after the pipeline is triggered.',
    requiredFor: 'Demo Execution',
    status: 'optional',
  },
]

const initialYaml = `pipeline:
  name: go-shortlink CI
  identifier: go_shortlink_ci
  projectIdentifier: harness_agent_studio
  orgIdentifier: default
  stages:
    - stage:
        name: Build and Publish
        type: CI
        spec:
          cloneCodebase: true
          execution:
            steps:
              - step:
                  type: Run
                  name: Go Test
                  spec:
                    command: go test ./...
              - step:
                  type: Run
                  name: Go Build
                  spec:
                    command: go build -o bin/app ./...
              - step:
                  type: BuildAndPushDockerRegistry
                  name: Docker Build and Push
`

const routes: RoutePath[] = ['/', '/preview', '/demo', '/connected']

function App() {
  const [route, setRoute] = useState<RoutePath>(getRoutePath())
  const [form, setForm] = useState<PipelineForm>({
    ...defaultForm,
    mode: getModeFromRoute(getRoutePath()),
  })
  const [connection, setConnection] = useState<HarnessConnection>({ accountId: '', apiToken: '' })
  const [yaml, setYaml] = useState(initialYaml)
  const [steps, setSteps] = useState<PipelineStep[]>(initialSteps)
  const [analysis, setAnalysis] = useState(initialAnalysis)
  const [recommendations, setRecommendations] = useState(initialRecommendations)
  const [missingSetup, setMissingSetup] = useState<SetupItem[]>(
    getStarterSetup(getModeFromRoute(getRoutePath())),
  )
  const [pipelineId, setPipelineId] = useState('go_shortlink_ci')
  const [status, setStatus] = useState<RequestStatus>('idle')
  const [message, setMessage] = useState('Ready')
  const [activeAction, setActiveAction] = useState<'analyze' | 'generate'>()

  useEffect(() => {
    function syncRoute() {
      const nextRoute = getRoutePath()
      const nextMode = getModeFromRoute(nextRoute)
      setRoute(nextRoute)
      setForm((current) => ({ ...current, mode: nextMode }))
      setMissingSetup(getStarterSetup(nextMode))
      setStatus('idle')
      setMessage('Ready')
    }

    window.addEventListener('popstate', syncRoute)
    return () => window.removeEventListener('popstate', syncRoute)
  }, [])

  async function handleAnalyze() {
    setStatus('loading')
    setMessage('Analyzing')
    setActiveAction('analyze')

    try {
      const data = await analyzeRepository(form, connection)
      setForm((current) => ({
        ...data.defaults,
        mode: current.mode,
        harnessAccountId: connection.accountId,
        hasHarnessToken: connection.apiToken.trim().length > 0,
      }))
      setAnalysis(data.analysis)
      setRecommendations(data.recommendations)
      setMissingSetup(data.missingSetup)
      setStatus('success')
      setMessage('Analyzed')
    } catch (error) {
      setStatus('error')
      setMessage(error instanceof Error ? error.message : 'Failed to analyze')
    } finally {
      setActiveAction(undefined)
    }
  }

  async function handleGenerate() {
    setStatus('loading')
    setMessage('Generating')
    setActiveAction('generate')

    try {
      const data = await generatePipeline(form, connection)
      setYaml(data.yaml)
      setSteps(data.nodes)
      setAnalysis(data.analysis)
      setRecommendations(data.recommendations)
      setMissingSetup(data.missingSetup)
      setPipelineId(data.pipelineIdentifier)
      setStatus('success')
      setMessage('Generated')
    } catch (error) {
      setStatus('error')
      setMessage(error instanceof Error ? error.message : 'Failed to generate')
    } finally {
      setActiveAction(undefined)
    }
  }

  async function handleCopy() {
    await navigator.clipboard.writeText(yaml)
    setMessage('Copied')
    setStatus('success')
  }

  function updateField<K extends keyof PipelineForm>(key: K, value: PipelineForm[K]) {
    setForm((current) => ({ ...current, [key]: value }))
  }

  function updateConnection<K extends keyof HarnessConnection>(
    key: K,
    value: HarnessConnection[K],
  ) {
    setConnection((current) => ({ ...current, [key]: value }))
  }

  function updateMode(mode: AppMode) {
    setForm((current) => ({ ...current, mode }))
  }

  function navigate(path: RoutePath) {
    const nextMode = getModeFromRoute(path)

    window.history.pushState({}, '', path)
    setRoute(path)
    setForm((current) => ({ ...current, mode: nextMode }))
    setMissingSetup(getStarterSetup(nextMode))
    setStatus('idle')
    setMessage('Ready')
  }

  const pageProps: PageProps = {
    form,
    connection,
    status,
    activeAction,
    yaml,
    pipelineId,
    steps,
    analysis,
    recommendations,
    missingSetup,
    onNavigate: navigate,
    onModeChange: updateMode,
    onFieldChange: updateField,
    onConnectionChange: updateConnection,
    onAnalyze: handleAnalyze,
    onGenerate: handleGenerate,
    onCopy: handleCopy,
  }

  return (
    <main className="app-shell">
      <header className="app-header">
        <div className="brand-mark">
          <ShieldCheck size={22} />
        </div>
        <div className="brand-copy">
          <span>AI DevOps Workbench</span>
          <strong>Harness Agent Studio</strong>
        </div>
        <nav className="app-nav" aria-label="Primary navigation">
          <NavButton
            active={route === '/'}
            icon={<Home size={17} />}
            label="Start"
            onClick={() => navigate('/')}
          />
          <NavButton
            active={route === '/preview'}
            label="Preview"
            onClick={() => navigate('/preview')}
          />
          <NavButton active={route === '/demo'} label="Demo" onClick={() => navigate('/demo')} />
          <NavButton
            active={route === '/connected'}
            label="Connected"
            onClick={() => navigate('/connected')}
          />
        </nav>
        <div className={`request-status ${status}`}>
          {status === 'error' ? <AlertCircle size={17} /> : <CheckCircle2 size={17} />}
          <span>{message}</span>
        </div>
      </header>

      {route === '/' && <HomePage {...pageProps} />}
      {route === '/preview' && <PreviewPage {...pageProps} />}
      {route === '/demo' && <DemoPage {...pageProps} />}
      {route === '/connected' && <ConnectedPage {...pageProps} />}
    </main>
  )
}

function NavButton({
  active,
  icon,
  label,
  onClick,
}: {
  active: boolean
  icon?: ReactNode
  label: string
  onClick: () => void
}) {
  return (
    <button className={`nav-button ${active ? 'active' : ''}`} type="button" onClick={onClick}>
      {icon ?? <Route size={17} />}
      <span>{label}</span>
    </button>
  )
}

function getRoutePath(): RoutePath {
  const path = window.location.pathname as RoutePath
  return routes.includes(path) ? path : '/'
}

function getModeFromRoute(path: RoutePath): AppMode {
  if (path === '/demo') {
    return 'demo'
  }

  if (path === '/connected') {
    return 'connected'
  }

  return 'preview'
}

function getStarterSetup(mode: AppMode): SetupItem[] {
  if (mode === 'demo') {
    return initialDemoSetup
  }

  return initialMissingSetup
}

export default App
