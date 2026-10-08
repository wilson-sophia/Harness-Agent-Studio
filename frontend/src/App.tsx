import { useEffect, useState } from 'react'
import { Activity, ArrowRight, CheckCircle2, CircleAlert, FlaskConical, GitBranch, Home, KeyRound, LogOut, Play, RefreshCw, Search, ShieldCheck } from 'lucide-react'
import { Field, InsightsPanel, ModeBadge, PipelineGraph, ProjectFormPanel, RepositorySummary, SetupChecklist, YamlPanel } from './components'
import * as api from './api'
import type { AppMode, DemoConfig, HarnessResource, PipelineForm, PipelineStep, RepoSignals, RoutePath, RunRecord, SessionInfo, SetupItem } from './types'
import './App.css'

const emptyForm: PipelineForm = {
  serviceName: '', repoUrl: '', projectDescription: '', branch: 'main', imageName: '',
  mode: 'preview', harnessAccountId: '', hasHarnessToken: false, orgIdentifier: '',
  projectIdentifier: '', codebaseConnectorRef: '', dockerConnectorRef: '',
  includeDeploy: false, k8sConnectorRef: '', namespace: 'default',
}
const routes: RoutePath[] = ['/', '/preview', '/demo', '/connected']
function routeFromLocation(): RoutePath {
  return routes.includes(location.pathname as RoutePath) ? location.pathname as RoutePath : '/'
}
function modeFor(route: RoutePath): AppMode {
  return route === '/demo' ? 'demo' : route === '/connected' ? 'connected' : 'preview'
}

function App() {
  const [route, setRoute] = useState<RoutePath>(routeFromLocation)
  const [form, setForm] = useState<PipelineForm>({ ...emptyForm, mode: modeFor(routeFromLocation()) })
  const [session, setSession] = useState<SessionInfo>({})
  const [authLoading, setAuthLoading] = useState(true)
  const [authMode, setAuthMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [remember, setRemember] = useState(true)
  const [freshPassword, setFreshPassword] = useState('')
  const [freshRequired, setFreshRequired] = useState(false)
  const [accountId, setAccountId] = useState('')
  const [token, setToken] = useState('')
  const [connection, setConnection] = useState<{ connected: boolean; accountId: string; encryptionConfigured: boolean }>()
  const [demoConfig, setDemoConfig] = useState<DemoConfig>()
  const [orgs, setOrgs] = useState<HarnessResource[]>([])
  const [projects, setProjects] = useState<HarnessResource[]>([])
  const [connectors, setConnectors] = useState<HarnessResource[]>([])
  const [signals, setSignals] = useState<RepoSignals>()
  const [analysisSource, setAnalysisSource] = useState('')
  const [analysis, setAnalysis] = useState<string[]>([])
  const [recommendations, setRecommendations] = useState<string[]>([])
  const [missingSetup, setMissingSetup] = useState<SetupItem[]>([])
  const [yaml, setYaml] = useState('')
  const [steps, setSteps] = useState<PipelineStep[]>([])
  const [pipelineId, setPipelineId] = useState('')
  const [runs, setRuns] = useState<RunRecord[]>([])
  const [runStatuses, setRunStatuses] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  function resetResults() {
    setSignals(undefined)
    setAnalysis([])
    setRecommendations([])
    setMissingSetup([])
    setYaml('')
    setSteps([])
    setPipelineId('')
  }

  useEffect(() => {
    void api.getSession().then(setSession).catch(() => setError('Backend is unavailable. Start the Go server on port 8080.')).finally(() => setAuthLoading(false))
    void api.getDemoConfig().then(setDemoConfig).catch(() => {})
    const onPop = () => {
      const next = routeFromLocation()
      setRoute(next)
      setForm((current) => ({ ...current, mode: modeFor(next) }))
      resetResults()
    }
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [])

  useEffect(() => {
    if (!session.authenticated) return
    let active = true
    Promise.all([api.getConnection(), api.getRuns()])
      .then(async ([connectionResult, runResult]) => {
        if (!active) return
        setConnection(connectionResult)
        setAccountId(connectionResult.accountId)
        setRuns(runResult)
        if (connectionResult.connected) {
          const result = await api.getResources('orgs')
          if (active) setOrgs(api.resourceOptions(result))
        }
      })
      .catch((cause) => { if (active) setError(errorText(cause)) })
    return () => { active = false }
  }, [session.authenticated])

  function navigate(next: RoutePath) {
    history.pushState({}, '', next)
    if (modeFor(next) !== modeFor(route)) resetResults()
    setRoute(next)
    setForm((current) => ({ ...current, mode: modeFor(next) }))
    setError('')
    setMessage('')
  }
  function updateField<K extends keyof PipelineForm>(key: K, value: PipelineForm[K]) {
    setForm((current) => ({ ...current, [key]: value }))
    setYaml('')
    setSteps([])
    if (key === 'repoUrl' || key === 'branch') {
      setSignals(undefined); setAnalysis([]); setRecommendations([])
    }
  }
  async function act(label: string, operation: () => Promise<void>) {
    setBusy(label); setError(''); setMessage('')
    try { await operation(); setMessage(label + ' complete') }
    catch (cause) {
      if (cause instanceof api.ApiError && cause.code === 'FRESH_AUTH_REQUIRED') setFreshRequired(true)
      if (cause instanceof api.ApiError && cause.status === 401) {
        void api.getSession().then(setSession).catch(() => setSession({ authenticated: false }))
      }
      setError(errorText(cause))
    } finally { setBusy('') }
  }
  async function handleAuth() {
    await act(authMode === 'register' ? 'Registration' : 'Sign in', async () => {
      const result = await api.authenticate(email, password, remember, authMode === 'register')
      setPassword(''); setSession({ ...result, authenticated: true })
    })
  }
  async function handleReauth() {
    await act('Password confirmation', async () => {
      const result = await api.confirmPassword(freshPassword)
      setSession((current) => ({ ...current, freshUntil: result.freshUntil }))
      setFreshPassword(''); setFreshRequired(false)
    })
  }
  async function handleAnalyze() {
    await act('Repository analysis', async () => {
      const result = await api.analyzeRepository(form)
      setForm((current) => ({
        ...current, ...result.defaults, mode: current.mode,
        orgIdentifier: current.orgIdentifier, projectIdentifier: current.projectIdentifier,
        codebaseConnectorRef: current.codebaseConnectorRef,
        dockerConnectorRef: current.dockerConnectorRef, k8sConnectorRef: current.k8sConnectorRef,
      }))
      setSignals(result.signals); setAnalysisSource(result.analysisSource)
      setAnalysis(result.analysis); setRecommendations(result.recommendations)
      setMissingSetup(result.missingSetup); setYaml(''); setSteps([])
    })
  }
  async function handleGenerate() {
    await act('YAML generation', async () => {
      const result = await api.generatePipeline(form)
      setYaml(result.yaml); setSteps(result.nodes); setPipelineId(result.pipelineIdentifier)
      setAnalysis(result.analysis); setRecommendations(result.recommendations)
      setMissingSetup(result.missingSetup)
    })
  }
  async function loadProjects(org: string) {
    setForm((current) => ({ ...current, orgIdentifier: org, projectIdentifier: '' }))
    setYaml('')
    if (!org) { setProjects([]); return }
    await act('Project discovery', async () => setProjects(api.resourceOptions(await api.getResources('projects', org))))
  }
  async function loadConnectors(project: string) {
    setForm((current) => ({ ...current, projectIdentifier: project }))
    setYaml('')
    if (!project) { setConnectors([]); return }
    await act('Connector discovery', async () => setConnectors(api.resourceOptions(await api.getResources('connectors', form.orgIdentifier, project))))
  }
  async function handleSaveConnection() {
    await act('Harness connection', async () => {
      const result = await api.saveConnection(accountId, token)
      setToken('')
      setConnection({ ...result, encryptionConfigured: true })
      setOrgs(api.resourceOptions(await api.getResources('orgs')))
    })
  }
  async function handleCreate() {
    await act('Pipeline creation', async () => {
      const record = await api.createPipeline(form)
      setRuns((current) => [record, ...current])
    })
  }
  async function handleTrigger(id: string) {
    await act('Pipeline execution', async () => {
      const result = await api.triggerRun(id)
      setRuns((current) => current.some((item) => item.id === result.run.id)
        ? current.map((item) => item.id === id ? result.run : item)
        : [result.run, ...current])
      setRunStatuses((current) => ({ ...current, [result.run.id]: result.status }))
    })
  }
  async function handleDemo() {
    await act('Demo execution', async () => {
      const result = await api.startDemo()
      setRuns((current) => [result.run, ...current])
      setRunStatuses((current) => ({ ...current, [result.run.id]: result.status }))
    })
  }
  async function refreshRun(id: string) {
    await act('Run status', async () => {
      const result = await api.getRunStatus(id)
      setRunStatuses((current) => ({ ...current, [id]: result.status }))
    })
  }
  const currentRuns = runs.filter((item) => item.mode === modeFor(route))

  return <main className="app-shell">
    <header className="app-header">
      <div className="brand-mark"><ShieldCheck size={22} /></div>
      <div className="brand-copy"><span>DEVOPS WORKBENCH</span><strong>Harness Agent Studio</strong></div>
      <nav className="app-nav" aria-label="Primary navigation">
        {([['/', 'Start', Home], ['/preview', 'Preview', Search], ['/demo', 'Demo', FlaskConical], ['/connected', 'Connected', KeyRound]] as const).map(([path, label, Icon]) =>
          <button key={path} type="button" className={`nav-button ${route === path ? 'active' : ''}`} onClick={() => navigate(path)}><Icon size={16} />{label}</button>)}
      </nav>
      {session.authenticated && <button className="nav-button" type="button" title="Sign out" onClick={() => void act('Sign out', async () => { await api.logout(); setSession({ authenticated: false }); setConnection(undefined); setRuns([]) })}><LogOut size={16} /><span>{session.email}</span></button>}
    </header>

    <div className="app-feedback" role="status" aria-live="polite">
      {busy && <span><RefreshCw size={15} className="spin-icon" />{busy}...</span>}
      {message && !busy && <span className="feedback-success"><CheckCircle2 size={15} />{message}</span>}
      {error && <span className="feedback-error"><CircleAlert size={15} />{error}</span>}
    </div>

    {route === '/' && <section className="home-page">
      <div className="home-intake panel">
        <div className="home-title"><span>Harness Agent Studio</span><h1>从项目地址，走到可验证的 Harness 流水线</h1></div>
        <div className="home-fields">
          <Field label="GitHub 仓库地址" value={form.repoUrl} placeholder="https://github.com/owner/repo" onChange={(value) => updateField('repoUrl', value)} />
          <Field label="分支" value={form.branch} onChange={(value) => updateField('branch', value)} />
        </div>
      </div>
      <div className="mode-grid">
        {([
          { path: '/preview', icon: Search, title: '预览方案', detail: '扫描公开仓库，生成分析、配置清单与 YAML。' },
          { path: '/demo', icon: FlaskConical, title: '运行示例', detail: '触发平台固定的 Harness Sandbox 示例。' },
          { path: '/connected', icon: KeyRound, title: '连接我的 Harness', detail: '用个人账号创建、触发并查看自己的流水线。' },
        ] as const).map(({ path, icon: Icon, title, detail }) =>
          <button className="path-card" type="button" key={path} onClick={() => navigate(path)}><span className="path-icon"><Icon size={22} /></span><span className="path-copy"><strong>{title}</strong><em>{detail}</em></span><ArrowRight size={18} /></button>)}
      </div>
    </section>}

    {route !== '/' && <section className="mode-page">
      <div className="mode-page-header"><div><span>REPOSITORY TO PIPELINE</span><h1>{route === '/preview' ? 'Preview Mode' : route === '/demo' ? 'Demo Mode' : 'Connected Mode'}</h1></div><ModeBadge mode={modeFor(route)} /></div>
      {route !== '/demo' && <RepositorySummary form={form} />}
      {route === '/connected' && !session.authenticated && <section className="auth-panel panel">
        <h2>{authMode === 'login' ? '登录后连接 Harness' : '创建账号'}</h2>
        <p>连接信息只保存在你的账户中，登录会话由 HttpOnly Cookie 管理。</p>
        <form onSubmit={(event) => { event.preventDefault(); void handleAuth() }}>
          <Field label="邮箱" type="email" value={email} onChange={setEmail} />
          <Field label="密码" type="password" value={password} onChange={setPassword} />
          <label className="toggle-row"><input type="checkbox" checked={remember} onChange={(event) => setRemember(event.target.checked)} /><span>在这台设备上保持登录</span></label>
          <button className="primary-action" disabled={!!busy || authLoading} type="submit">{authMode === 'login' ? '登录' : '注册'}</button>
        </form>
        <button className="text-action" type="button" onClick={() => setAuthMode(authMode === 'login' ? 'register' : 'login')}>{authMode === 'login' ? '没有账号？创建账号' : '已有账号？返回登录'}</button>
      </section>}

      {(route !== '/connected' || session.authenticated) && <>
        {freshRequired && <section className="fresh-bar" role="alert"><KeyRound size={18} /><span>此操作需要最近 5 分钟内验证密码。</span><input aria-label="确认密码" type="password" value={freshPassword} onChange={(event) => setFreshPassword(event.target.value)} /><button className="secondary-action" type="button" onClick={() => void handleReauth()}>确认</button></section>}
        <div className="mode-layout">
          <div className="sidebar-stack">
            {route === '/connected' && <section className="panel connection-panel">
              <h2><KeyRound size={18} /> Harness 连接</h2>
              {!connection?.encryptionConfigured && <p className="field-note">后端尚未设置 APP_ENCRYPTION_KEY。设置后才能保存 API Token。</p>}
              {connection?.connected ? <div className="connected-state"><CheckCircle2 size={18} />已连接账号 {connection.accountId}<button className="text-action" type="button" onClick={() => void act('Disconnect', async () => { await api.deleteConnection(); setConnection({ connected: false, accountId: '', encryptionConfigured: true }); setOrgs([]); setProjects([]); setConnectors([]) })}>断开连接</button></div> :
                <><Field label="Harness Account ID" value={accountId} onChange={setAccountId} /><Field label="Harness API Token" type="password" value={token} onChange={setToken} /><button className="primary-action" type="button" disabled={!!busy || !accountId || !token} onClick={() => void handleSaveConnection()}>验证并连接</button></>}
            </section>}
            {route === '/demo' && <section className="panel demo-config"><h2><FlaskConical size={18} /> Sandbox</h2><p>{demoConfig?.configured ? '可运行固定示例仓库：' + demoConfig.repository : '未配置 Sandbox。请在后端设置 DEMO_* 环境变量。'}</p><button className="primary-action" type="button" disabled={!!busy || !demoConfig?.configured || !session.authenticated} onClick={() => void handleDemo()}><Play size={16} />运行示例</button>{!session.authenticated && <p>运行示例需要先在 Connected 页面登录。</p>}</section>}
            {route !== '/demo' && <ProjectFormPanel form={form} status={busy ? 'loading' : 'idle'} activeAction={busy === 'Repository analysis' ? 'analyze' : busy === 'YAML generation' ? 'generate' : undefined} onFieldChange={updateField} onAnalyze={() => void handleAnalyze()} onGenerate={() => void handleGenerate()} canGenerate={!!signals && (route !== '/connected' || (!!form.orgIdentifier && !!form.projectIdentifier && !!form.codebaseConnectorRef && !!form.k8sConnectorRef))} compact={route === '/connected'} />}
            {route === '/connected' && connection?.connected && <section className="panel resource-panel">
              <h2><GitBranch size={18} /> Harness 资源</h2>
              <label className="field"><span>Organization</span><select value={form.orgIdentifier} onChange={(event) => void loadProjects(event.target.value)}><option value="">选择组织</option>{orgs.map((item) => <option key={item.identifier} value={item.identifier}>{item.name}</option>)}</select></label>
              <label className="field"><span>Project</span><select value={form.projectIdentifier} onChange={(event) => void loadConnectors(event.target.value)}><option value="">选择项目</option>{projects.map((item) => <option key={item.identifier} value={item.identifier}>{item.name}</option>)}</select></label>
              <label className="field"><span>Git connector</span><select value={form.codebaseConnectorRef} onChange={(event) => updateField('codebaseConnectorRef', event.target.value)}><option value="">选择连接器</option>{connectors.map((item) => <option key={item.identifier} value={item.identifier}>{item.name} {item.type && '(' + item.type + ')'}</option>)}</select></label>
              <label className="field"><span>Docker connector</span><select value={form.dockerConnectorRef} onChange={(event) => updateField('dockerConnectorRef', event.target.value)}><option value="">不发布镜像</option>{connectors.map((item) => <option key={item.identifier} value={item.identifier}>{item.name}</option>)}</select></label>
              <label className="field"><span>CI infrastructure connector</span><select value={form.k8sConnectorRef} onChange={(event) => updateField('k8sConnectorRef', event.target.value)}><option value="">选择运行环境</option>{connectors.map((item) => <option key={item.identifier} value={item.identifier}>{item.name}</option>)}</select></label>
            </section>}
          </div>
          <section className="mode-main">
            {route !== '/demo' && <>
              {signals && <section className="scan-strip"><strong>{signals.language.toUpperCase()}</strong><span>{signals.workdir || '根目录'}</span><span>{signals.hasTests ? '发现测试' : '未发现测试'}</span><span>{signals.hasDockerfile ? '发现 Dockerfile' : '未发现 Dockerfile'}</span><small>{analysisSource === 'model' ? '模型建议' : '规则建议'}</small></section>}
              {analysis.length > 0 ? <div className="result-grid"><InsightsPanel analysis={analysis} recommendations={recommendations} /><SetupChecklist items={missingSetup} /></div> : <section className="empty-result"><Search size={24} /><h2>等待仓库分析</h2><p>输入公开 GitHub 仓库和分支，点击“Analyze”查看真实扫描结果。</p></section>}
              {steps.length > 0 && <PipelineGraph steps={steps} />}
              {yaml && <YamlPanel pipelineId={pipelineId} yaml={yaml} onCopy={() => void navigator.clipboard.writeText(yaml).then(() => setMessage('YAML copied'))} />}
            </>}
            {route === '/demo' && currentRuns.length === 0 && <section className="empty-result"><FlaskConical size={24} /><h2>没有 Sandbox 运行</h2><p>配置固定示例流水线后，可在左侧启动并查看状态。</p></section>}
            {route === '/connected' && <section className="panel runs-panel">
              <div className="runs-heading"><h2><Activity size={18} />我的流水线</h2><button className="primary-action compact-action" type="button" disabled={!!busy || !yaml || !connection?.connected} onClick={() => void handleCreate()}>在 Harness 创建</button></div>
              {currentRuns.length === 0 ? <p>创建流水线后，运行记录会显示在这里。</p> : currentRuns.map((run) => <RunItem key={run.id} run={run} status={runStatuses[run.id]} busy={!!busy} onTrigger={() => void handleTrigger(run.id)} onRefresh={() => void refreshRun(run.id)} />)}
            </section>}
            {route === '/demo' && currentRuns.length > 0 && <section className="panel runs-panel"><h2><Activity size={18} />示例运行</h2>{currentRuns.map((run) => <RunItem key={run.id} run={run} status={runStatuses[run.id]} busy={!!busy} onRefresh={() => void refreshRun(run.id)} />)}</section>}
          </section>
        </div>
      </>}
    </section>}
  </main>
}

function RunItem({ run, status, busy, onTrigger, onRefresh }: { run: RunRecord; status?: string; busy: boolean; onTrigger?: () => void; onRefresh: () => void }) {
  return <article className="run-row">
    <div><strong>{run.pipelineIdentifier}</strong><small>{run.executionId ? 'Execution ' + run.executionId : 'Created, not started'}</small><span>{status || (run.executionId ? '状态待查询' : '未运行')}</span></div>
    <div className="run-actions">
      {onTrigger && <button className="secondary-action compact-action" type="button" disabled={busy} onClick={onTrigger}><Play size={16} />{run.executionId ? '再次运行' : '运行'}</button>}
      {run.executionId && <button className="icon-button" title="刷新运行状态" type="button" disabled={busy} onClick={onRefresh}><RefreshCw size={16} /></button>}
      {run.executionId && <a className="text-action" target="_blank" rel="noreferrer" href={`https://app.harness.io/ng/account/${encodeURIComponent(run.accountId)}/module/ci/orgs/${encodeURIComponent(run.orgIdentifier)}/projects/${encodeURIComponent(run.projectIdentifier)}/pipelines/${encodeURIComponent(run.pipelineIdentifier)}/executions/${encodeURIComponent(run.executionId)}/pipeline`}>Harness 详情</a>}
    </div>
  </article>
}

function errorText(value: unknown): string { return value instanceof Error ? value.message : '操作失败，请重试' }
export default App
