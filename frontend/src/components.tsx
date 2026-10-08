import { useMemo, type ReactNode } from 'react'
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type Edge,
  type Node,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  BrainCircuit,
  Copy,
  GitBranch,
  Lightbulb,
  ListChecks,
  PackageCheck,
  Server,
  SlidersHorizontal,
} from 'lucide-react'
import type {
  AppMode,
  HarnessConnection,
  PipelineForm,
  PipelineStep,
  RequestStatus,
  SetupItem,
} from './types'

type FieldProps = {
  label: string
  value: string
  type?: string
  placeholder?: string
  onChange: (value: string) => void
}

type FormPanelProps = {
  form: PipelineForm
  connection?: HarnessConnection
  status: RequestStatus
  activeAction?: 'analyze' | 'generate'
  onFieldChange: <K extends keyof PipelineForm>(key: K, value: PipelineForm[K]) => void
  onConnectionChange?: <K extends keyof HarnessConnection>(
    key: K,
    value: HarnessConnection[K],
  ) => void
  onAnalyze: () => void
  onGenerate: () => void
  canGenerate?: boolean
  compact?: boolean
  showConnection?: boolean
  showTargets?: boolean
}

const stepTone: Record<string, { border: string; background: string; color: string }> = {
  source: { border: '#2f68d8', background: '#edf4ff', color: '#0b3b91' },
  test: { border: '#12a37f', background: '#eafaf5', color: '#075944' },
  build: { border: '#d97706', background: '#fff7ed', color: '#7c2d12' },
  package: { border: '#7c3aed', background: '#f5f3ff', color: '#4c1d95' },
  deploy: { border: '#dc2626', background: '#fef2f2', color: '#7f1d1d' },
}

export function Field({
  label,
  value,
  type = 'text',
  placeholder,
  onChange,
}: FieldProps) {
  return (
    <label className="field">
      <span>{label}</span>
      <input
        placeholder={placeholder}
        type={type}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  )
}

export function ProjectFormPanel({
  form,
  connection,
  status,
  activeAction,
  onFieldChange,
  onConnectionChange,
  onAnalyze,
  onGenerate,
  canGenerate = true,
  compact = false,
  showConnection = false,
  showTargets = false,
}: FormPanelProps) {
  return (
    <aside className="panel form-panel">
      <SectionHeading icon={<Server size={18} />} title="Project" />
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

      {showConnection && connection && onConnectionChange && (
        <section className="soft-box">
          <SectionHeading icon={<SlidersHorizontal size={17} />} title="Harness Connection" />
          <Field
            label="Account ID"
            value={connection.accountId}
            onChange={(value) => onConnectionChange('accountId', value)}
          />
          <Field
            label="API Token"
            type="password"
            value={connection.apiToken}
            onChange={(value) => onConnectionChange('apiToken', value)}
          />
          <p className="field-note">Token value is not sent to the analyzer request.</p>
        </section>
      )}

      <div className="button-stack">
        <button className="secondary-action" type="button" disabled={status === 'loading' || !form.repoUrl.trim()} onClick={onAnalyze}>
          {activeAction === 'analyze' && status === 'loading' ? (
            <span className="loader" />
          ) : (
            <GitBranch size={18} />
          )}
          <span>Analyze</span>
        </button>
        <button className="primary-action" type="button" disabled={status === 'loading' || !canGenerate} onClick={onGenerate}>
          {activeAction === 'generate' && status === 'loading' ? (
            <span className="loader light" />
          ) : (
            <PackageCheck size={18} />
          )}
          <span>Generate YAML</span>
        </button>
      </div>

      {!compact && (
        <>
          <SectionHeading icon={<Lightbulb size={18} />} title="Generated Defaults" />
          <Field
            label="Service"
            value={form.serviceName}
            onChange={(value) => onFieldChange('serviceName', value)}
          />
          <Field
            label="Image"
            value={form.imageName}
            onChange={(value) => onFieldChange('imageName', value)}
          />
          <div className="field"><span>Detected Context</span><p className="field-note">{form.projectDescription || 'Analyze the repository to detect its build context.'}</p></div>
        </>
      )}

      {showTargets && (
        <section className="soft-box target-box">
          <SectionHeading icon={<SlidersHorizontal size={17} />} title="Harness Target" />
          <Field
            label="Org"
            value={form.orgIdentifier}
            onChange={(value) => onFieldChange('orgIdentifier', value)}
          />
          <Field
            label="Project"
            value={form.projectIdentifier}
            onChange={(value) => onFieldChange('projectIdentifier', value)}
          />
          <Field
            label="Git Connector"
            value={form.codebaseConnectorRef}
            onChange={(value) => onFieldChange('codebaseConnectorRef', value)}
          />
          <Field
            label="Docker Connector"
            value={form.dockerConnectorRef}
            onChange={(value) => onFieldChange('dockerConnectorRef', value)}
          />
        </section>
      )}

    </aside>
  )
}

export function InsightsPanel({
  analysis,
  recommendations,
}: {
  analysis: string[]
  recommendations: string[]
}) {
  return (
    <section className="panel insights-panel">
      <InsightList icon={<BrainCircuit size={18} />} title="Agent Analysis" items={analysis} />
      <InsightList icon={<Lightbulb size={18} />} title="Recommendations" items={recommendations} />
    </section>
  )
}

export function InsightList({
  icon,
  title,
  items,
}: {
  icon: ReactNode
  title: string
  items: string[]
}) {
  return (
    <section className="insight-block">
      <div className="insight-title">
        {icon}
        <h3>{title}</h3>
      </div>
      <ul>
        {items.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
    </section>
  )
}

export function SetupChecklist({ items }: { items: SetupItem[] }) {
  return (
    <section className="panel setup-panel">
      <SectionHeading icon={<ListChecks size={18} />} title="Missing Setup" />
      <div className="setup-grid">
        {items.map((item) => (
          <article className={`setup-item ${item.status}`} key={item.id}>
            <div>
              <strong>{item.label}</strong>
              <span>{item.requiredFor}</span>
            </div>
            <p>{item.description}</p>
          </article>
        ))}
      </div>
    </section>
  )
}

export function PipelineGraph({ steps }: { steps: PipelineStep[] }) {
  const flow = useMemo(() => buildFlow(steps), [steps])

  return (
    <section className="panel graph-panel">
      <SectionHeading icon={<PackageCheck size={18} />} title="Pipeline Preview" />
      <div className="flow-canvas">
        <ReactFlow
          nodes={flow.nodes}
          edges={flow.edges}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable={false}
        >
          <Background color="#d7dde8" gap={18} />
          <MiniMap pannable={false} zoomable={false} />
          <Controls showInteractive={false} />
        </ReactFlow>
      </div>

      <div className="step-list">
        {steps.map((step) => (
          <div className="step-row" key={step.id}>
            <span className={`step-dot ${step.kind}`} />
            <div>
              <strong>{step.label}</strong>
              <p>{step.command ?? step.description}</p>
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

export function YamlPanel({
  pipelineId,
  yaml,
  onCopy,
}: {
  pipelineId: string
  yaml: string
  onCopy: () => void
}) {
  return (
    <aside className="panel yaml-panel">
      <div className="section-heading split-heading">
        <div>
          <h2>YAML</h2>
          <p>{pipelineId}</p>
        </div>
        <button className="icon-button" type="button" onClick={onCopy} aria-label="Copy YAML">
          <Copy size={18} />
        </button>
      </div>
      <pre className="yaml-output">{yaml}</pre>
    </aside>
  )
}

export function ModeBadge({ mode }: { mode: AppMode }) {
  const label = mode === 'preview' ? 'Preview' : mode === 'demo' ? 'Demo' : 'Connected'
  return <span className={`mode-pill ${mode}`}>{label}</span>
}

export function SectionHeading({
  icon,
  title,
  subtitle,
}: {
  icon: ReactNode
  title: string
  subtitle?: string
}) {
  return (
    <div className="section-heading">
      {icon}
      <div>
        <h2>{title}</h2>
        {subtitle && <p>{subtitle}</p>}
      </div>
    </div>
  )
}

export function RepositorySummary({ form }: { form: PipelineForm }) {
  return (
    <div className="repo-summary">
      <span>{form.serviceName || 'Service'}</span>
      <strong>{form.repoUrl}</strong>
      <small>{form.branch}</small>
    </div>
  )
}

function buildFlow(steps: PipelineStep[]): { nodes: Node[]; edges: Edge[] } {
  const nodes = steps.map((step, index) => {
    const tone = stepTone[step.kind] ?? stepTone.source

    return {
      id: step.id,
      position: { x: index * 230, y: index % 2 === 0 ? 80 : 190 },
      data: {
        label: (
          <div className="flow-node">
            <strong>{step.label}</strong>
            <span>{step.command ?? step.description}</span>
          </div>
        ),
      },
      style: {
        width: 190,
        minHeight: 92,
        border: `1px solid ${tone.border}`,
        background: tone.background,
        color: tone.color,
        borderRadius: 8,
        boxShadow: '0 14px 30px rgba(15, 23, 42, 0.08)',
      },
    }
  })

  const edges: Edge[] = steps.slice(1).map((step, index) => ({
    id: `${steps[index].id}-${step.id}`,
    source: steps[index].id,
    target: step.id,
    animated: true,
    style: { stroke: '#61738f', strokeWidth: 2 },
  }))

  return { nodes, edges }
}
