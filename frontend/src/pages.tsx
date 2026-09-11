import {
  ArrowRight,
  Boxes,
  FlaskConical,
  KeyRound,
  LockKeyhole,
  PackageCheck,
  ShieldCheck,
  Sparkles,
} from 'lucide-react'
import {
  Field,
  InsightsPanel,
  ModeBadge,
  PipelineGraph,
  ProjectFormPanel,
  RepositorySummary,
  SetupChecklist,
  YamlPanel,
} from './components'
import type {
  AppMode,
  HarnessConnection,
  PipelineForm,
  PipelineStep,
  RequestStatus,
  RoutePath,
  SetupItem,
} from './types'

export type PageProps = {
  form: PipelineForm
  connection: HarnessConnection
  status: RequestStatus
  activeAction?: 'analyze' | 'generate'
  yaml: string
  pipelineId: string
  steps: PipelineStep[]
  analysis: string[]
  recommendations: string[]
  missingSetup: SetupItem[]
  onNavigate: (path: RoutePath) => void
  onModeChange: (mode: AppMode) => void
  onFieldChange: <K extends keyof PipelineForm>(key: K, value: PipelineForm[K]) => void
  onConnectionChange: <K extends keyof HarnessConnection>(
    key: K,
    value: HarnessConnection[K],
  ) => void
  onAnalyze: () => void
  onGenerate: () => void
  onCopy: () => void
}

const modeCards: Array<{
  mode: AppMode
  path: RoutePath
  title: string
  badge: string
  description: string
  icon: typeof ShieldCheck
}> = [
  {
    mode: 'preview',
    path: '/preview',
    title: 'Preview Mode',
    badge: 'No credentials',
    description: 'Repo analysis, Harness YAML draft, and missing setup map.',
    icon: ShieldCheck,
  },
  {
    mode: 'demo',
    path: '/demo',
    title: 'Demo Mode',
    badge: 'Sandbox',
    description: 'Platform-owned Harness demo path for trusted repositories.',
    icon: FlaskConical,
  },
  {
    mode: 'connected',
    path: '/connected',
    title: 'Connected Mode',
    badge: 'User Harness',
    description: 'Authorized Harness account setup, target selection, and pipeline creation path.',
    icon: KeyRound,
  },
]

export function HomePage({
  form,
  onNavigate,
  onModeChange,
  onFieldChange,
}: PageProps) {
  function openMode(mode: AppMode, path: RoutePath) {
    onModeChange(mode)
    onNavigate(path)
  }

  return (
    <section className="home-page">
      <div className="home-intake panel">
        <div className="home-title">
          <span>Harness Agent Studio</span>
          <h1>Route a GitHub service into the right Harness workflow.</h1>
        </div>
        <div className="home-fields">
          <Field
            label="Repository"
            value={form.repoUrl}
            onChange={(value) => onFieldChange('repoUrl', value)}
          />
          <Field
            label="Branch"
            value={form.branch}
            onChange={(value) => onFieldChange('branch', value)}
          />
        </div>
      </div>

      <div className="mode-grid">
        {modeCards.map((card) => {
          const Icon = card.icon

          return (
            <button
              className={`path-card ${form.mode === card.mode ? 'active' : ''}`}
              key={card.mode}
              type="button"
              onClick={() => openMode(card.mode, card.path)}
            >
              <span className="path-icon">
                <Icon size={22} />
              </span>
              <span className="path-copy">
                <small>{card.badge}</small>
                <strong>{card.title}</strong>
                <em>{card.description}</em>
              </span>
              <ArrowRight size={18} />
            </button>
          )
        })}
      </div>

      <section className="home-map panel">
        <div>
          <Sparkles size={20} />
          <h2>Interview Story</h2>
        </div>
        <p>
          The app separates low-friction preview, safe sandbox demo, and real user-connected
          Harness operations into different product paths.
        </p>
      </section>
    </section>
  )
}

export function PreviewPage(props: PageProps) {
  return (
    <ModePageShell
      eyebrow="No credentials"
      title="Preview Mode"
      mode="preview"
      form={props.form}
      aside={
        <ProjectFormPanel
          form={props.form}
          status={props.status}
          activeAction={props.activeAction}
          onFieldChange={props.onFieldChange}
          onAnalyze={props.onAnalyze}
          onGenerate={props.onGenerate}
        />
      }
      main={
        <>
          <div className="result-grid">
            <InsightsPanel
              analysis={props.analysis}
              recommendations={props.recommendations}
            />
            <SetupChecklist items={props.missingSetup} />
          </div>
          <PipelineGraph steps={props.steps} />
          <YamlPanel pipelineId={props.pipelineId} yaml={props.yaml} onCopy={props.onCopy} />
        </>
      }
    />
  )
}

export function DemoPage(props: PageProps) {
  return (
    <ModePageShell
      eyebrow="Sandbox"
      title="Demo Mode"
      mode="demo"
      form={props.form}
      aside={
        <ProjectFormPanel
          form={props.form}
          status={props.status}
          activeAction={props.activeAction}
          onFieldChange={props.onFieldChange}
          onAnalyze={props.onAnalyze}
          onGenerate={props.onGenerate}
          compact
        />
      }
      main={
        <>
          <section className="demo-status">
            <article className="panel status-tile">
              <LockKeyhole size={19} />
              <strong>Whitelist</strong>
              <span>Sandbox-only execution boundary</span>
            </article>
            <article className="panel status-tile">
              <Boxes size={19} />
              <strong>Harness Project</strong>
              <span>Reusable demo org, project, and connectors</span>
            </article>
            <article className="panel status-tile">
              <PackageCheck size={19} />
              <strong>Pipeline Run</strong>
              <span>Milestone after preview generation is stable</span>
            </article>
          </section>
          <div className="result-grid">
            <InsightsPanel
              analysis={props.analysis}
              recommendations={props.recommendations}
            />
            <SetupChecklist items={props.missingSetup} />
          </div>
          <PipelineGraph steps={props.steps} />
          <YamlPanel pipelineId={props.pipelineId} yaml={props.yaml} onCopy={props.onCopy} />
        </>
      }
    />
  )
}

export function ConnectedPage(props: PageProps) {
  return (
    <ModePageShell
      eyebrow="User Harness"
      title="Connected Mode"
      mode="connected"
      form={props.form}
      aside={
        <ProjectFormPanel
          form={props.form}
          connection={props.connection}
          status={props.status}
          activeAction={props.activeAction}
          onFieldChange={props.onFieldChange}
          onConnectionChange={props.onConnectionChange}
          onAnalyze={props.onAnalyze}
          onGenerate={props.onGenerate}
          showConnection
          showTargets
        />
      }
      main={
        <>
          <section className="connected-overview panel">
            <div>
              <KeyRound size={20} />
              <h2>Authorized Harness Workspace</h2>
            </div>
            <p>
              Account, connector, and project fields become the execution contract for a future
              Harness API integration.
            </p>
          </section>
          <div className="result-grid">
            <InsightsPanel
              analysis={props.analysis}
              recommendations={props.recommendations}
            />
            <SetupChecklist items={props.missingSetup} />
          </div>
          <PipelineGraph steps={props.steps} />
          <YamlPanel pipelineId={props.pipelineId} yaml={props.yaml} onCopy={props.onCopy} />
        </>
      }
    />
  )
}

function ModePageShell({
  eyebrow,
  title,
  mode,
  form,
  aside,
  main,
}: {
  eyebrow: string
  title: string
  mode: AppMode
  form: PipelineForm
  aside: React.ReactNode
  main: React.ReactNode
}) {
  return (
    <section className="mode-page">
      <div className="mode-page-header">
        <div>
          <span>{eyebrow}</span>
          <h1>{title}</h1>
        </div>
        <ModeBadge mode={mode} />
      </div>
      <RepositorySummary form={form} />
      <div className="mode-layout">
        {aside}
        <section className="mode-main">{main}</section>
      </div>
    </section>
  )
}
