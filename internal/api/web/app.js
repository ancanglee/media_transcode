// API 基础路径
const API_BASE = '/api';

// 状态
let currentPage = 1;
let pageSize = 10;
let totalTasks = 0;

// 仪表盘任务列表状态
let dashboardTasksPage = 1;
let dashboardTasksTotal = 0;
let dashboardTasksStatus = '';

// 系统配置
let systemConfig = {
    inputBucket: '',
    outputBucket: ''
};

// AI 生成结果缓存
let currentAIResult = null;

// 当前用户信息
let currentUser = null;

// ==================== 认证相关 ====================

// 获取认证头
function getAuthHeaders() {
    const token = localStorage.getItem('auth_token');
    return {
        'Content-Type': 'application/json',
        'Authorization': token ? `Bearer ${token}` : ''
    };
}

// 带认证的 fetch
async function authFetch(url, options = {}) {
    const headers = getAuthHeaders();
    options.headers = { ...headers, ...options.headers };
    
    const res = await fetch(url, options);
    
    // 如果返回401，跳转到登录页
    if (res.status === 401) {
        localStorage.removeItem('auth_token');
        localStorage.removeItem('auth_user');
        window.location.href = '/login';
        return null;
    }
    
    return res;
}

// 检查登录状态
function checkAuth() {
    const token = localStorage.getItem('auth_token');
    const userStr = localStorage.getItem('auth_user');
    
    if (!token || !userStr) {
        window.location.href = '/login';
        return false;
    }
    
    try {
        currentUser = JSON.parse(userStr);
        document.getElementById('currentUser').textContent = `👤 ${currentUser.username}`;
        
        // 根据角色显示/隐藏管理员功能
        const adminTabs = document.querySelectorAll('.admin-only');
        adminTabs.forEach(tab => {
            tab.style.display = currentUser.role === 'admin' ? '' : 'none';
        });
        
        return true;
    } catch (e) {
        window.location.href = '/login';
        return false;
    }
}

// 退出登录
function logout() {
    localStorage.removeItem('auth_token');
    localStorage.removeItem('auth_user');
    window.location.href = '/login';
}

// 初始化
document.addEventListener('DOMContentLoaded', () => {
    // 先检查登录状态
    if (!checkAuth()) return;
    
    initTabs();
    initDateFilter();
    checkHealth();
    loadDashboard();
    loadPlatformInfo();
    loadPresets();
    loadTranscodeTypes();
    initTableResize();
    loadSystemConfig();
});

// 加载平台信息
async function loadPlatformInfo() {
    try {
        const res = await authFetch(`${API_BASE}/platform`);
        if (!res) return;
        const data = await res.json();
        const badge = document.getElementById('platformInfo');
        if (badge) {
            const gpuStatus = data.gpu_available ? '✅' : '⚠️';
            badge.textContent = `${gpuStatus} ${data.platform} | ${data.video_encoder}`;
            badge.className = `platform-badge ${data.gpu_available ? 'gpu-enabled' : 'cpu-mode'}`;
        }
    } catch (e) {
        console.error('加载平台信息失败:', e);
    }
}

// 加载系统配置
async function loadSystemConfig() {
    try {
        const res = await authFetch(`${API_BASE}/config`);
        if (!res) return;
        const data = await res.json();
        systemConfig.inputBucket = data.input_bucket || '';
        systemConfig.outputBucket = data.output_bucket || '';
        const inputBucketEl = document.getElementById('inputBucket');
        if (inputBucketEl && systemConfig.inputBucket) {
            inputBucketEl.value = systemConfig.inputBucket;
        }
    } catch (e) {
        console.error('加载系统配置失败:', e);
    }
}

// Tab 切换
function initTabs() {
    const tabs = document.querySelectorAll('.tab-btn');
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            tabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');
            document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
            document.getElementById(tab.dataset.tab).classList.add('active');
            if (tab.dataset.tab === 'task-queue') {
                loadTasks();
                loadQueueStats();
            } else if (tab.dataset.tab === 'presets') {
                loadPresets();
            } else if (tab.dataset.tab === 'users') {
                loadUsers();
            }
        });
    });
}

// 初始化日期筛选器
function initDateFilter() {
    const today = new Date().toISOString().split('T')[0];
    const dateFilter = document.getElementById('dateFilter');
    const statusFilter = document.getElementById('statusFilter');
    dateFilter.value = today;
    dateFilter.addEventListener('change', () => { currentPage = 1; loadTasks(); });
    statusFilter.addEventListener('change', () => { currentPage = 1; loadTasks(); });
}

// 健康检查
async function checkHealth() {
    const statusEl = document.getElementById('healthStatus');
    const dot = statusEl.querySelector('.status-dot');
    const text = statusEl.querySelector('.status-text');
    try {
        const res = await fetch(`${API_BASE}/health`);
        const data = await res.json();
        if (data.status === 'healthy') {
            dot.className = 'status-dot healthy';
            text.textContent = '服务正常';
        } else {
            dot.className = 'status-dot error';
            text.textContent = '服务异常';
        }
    } catch (e) {
        dot.className = 'status-dot error';
        text.textContent = '连接失败';
    }
}

// ==================== AI 智能转码功能 ====================

// 生成 FFmpeg 参数
async function generateFFmpegParams(event) {
    event.preventDefault();
    const requirement = document.getElementById('aiRequirement').value.trim();
    const inputFormat = document.getElementById('aiInputFormat').value.trim();
    const autoTest = document.getElementById('aiAutoTest').checked;
    const btn = document.getElementById('generateBtn');
    
    btn.disabled = true;
    if (autoTest) {
        btn.textContent = '⏳ 生成并测试中...';
    } else {
        btn.textContent = '⏳ 生成中...';
    }
    
    // 隐藏之前的测试结果
    document.getElementById('autoTestResult').style.display = 'none';
    
    try {
        const res = await authFetch(`${API_BASE}/llm/generate`, {
            method: 'POST',
            body: JSON.stringify({ requirement, input_format: inputFormat, auto_test: autoTest })
        });
        if (!res) return;
        const data = await res.json();
        
        if (!res.ok) {
            throw new Error(data.error || '生成失败');
        }
        
        currentAIResult = data;
        displayAIResult(data);
        
        // 显示自动测试结果
        if (autoTest && data.test_result) {
            displayAutoTestResult(data.test_result);
        }
        
        if (data.test_result && data.test_result.success) {
            showToast('参数生成并测试成功！可以保存为预设', 'success');
        } else if (data.test_result && !data.test_result.success) {
            showToast('参数生成成功，但测试失败，请检查或手动调整', 'error');
        } else {
            showToast('参数生成成功', 'success');
        }
    } catch (e) {
        showToast(`生成失败: ${e.message}`, 'error');
    } finally {
        btn.disabled = false;
        btn.textContent = '🚀 生成参数';
    }
}

// 显示自动测试结果
function displayAutoTestResult(testResult) {
    const container = document.getElementById('autoTestResult');
    const title = document.getElementById('autoTestTitle');
    const status = document.getElementById('autoTestStatus');
    const command = document.getElementById('autoTestCommand');
    const output = document.getElementById('autoTestOutput');
    
    container.style.display = 'block';
    
    if (testResult.success) {
        title.textContent = '✅ 自动测试通过';
        title.style.color = '#10b981';
        status.innerHTML = `<span class="status-badge status-completed">测试成功</span>` +
            (testResult.retries > 0 ? ` <span class="hint">（经过 ${testResult.retries} 次修正）</span>` : '');
    } else {
        title.textContent = '❌ 自动测试失败';
        title.style.color = '#ef4444';
        status.innerHTML = `<span class="status-badge status-failed">测试失败</span>` +
            ` <span class="hint">（已尝试 ${testResult.retries + 1} 次）</span>` +
            `<div class="error-message" style="margin-top:8px;color:#ef4444;">${testResult.error || '未知错误'}</div>`;
    }
    
    command.textContent = testResult.command || '无';
    output.textContent = testResult.output || '无输出';
}

// 显示 AI 生成结果
function displayAIResult(data) {
    document.getElementById('resultName').textContent = data.name;
    document.getElementById('resultDescription').textContent = data.description;
    document.getElementById('resultOutputExt').textContent = data.output_ext;
    document.getElementById('resultSpeed').textContent = data.estimated_speed || '-';
    document.getElementById('resultArgs').textContent = data.ffmpeg_args.join(' ');
    document.getElementById('resultExplanation').textContent = data.explanation;
    document.getElementById('aiResult').style.display = 'block';
}

// 最后一次测试的错误信息（用于修正）
let lastTestError = null;

// 测试 FFmpeg 参数
function testFFmpegParams() {
    if (!currentAIResult) {
        showToast('请先生成参数', 'error');
        return;
    }
    // 显示当前参数
    document.getElementById('currentTestArgs').textContent = currentAIResult.ffmpeg_args.join(' ');
    // 重置测试结果区域
    document.getElementById('testResult').style.display = 'none';
    document.getElementById('testFixSection').style.display = 'none';
    document.getElementById('testSuccessSection').style.display = 'none';
    lastTestError = null;
    
    document.getElementById('testModal').classList.add('active');
}

function closeTestModal() {
    document.getElementById('testModal').classList.remove('active');
}

// 运行测试
async function runTest() {
    const inputFile = document.getElementById('testInputFile').value.trim();
    if (!inputFile) {
        showToast('请输入测试文件路径', 'error');
        return;
    }
    
    const btn = document.getElementById('runTestBtn');
    btn.disabled = true;
    btn.textContent = '⏳ 测试中...';
    
    try {
        const res = await authFetch(`${API_BASE}/llm/test`, {
            method: 'POST',
            body: JSON.stringify({
                input_file: inputFile,
                ffmpeg_args: currentAIResult.ffmpeg_args,
                output_ext: currentAIResult.output_ext
            })
        });
        if (!res) return;
        const data = await res.json();
        
        document.getElementById('testResult').style.display = 'block';
        document.getElementById('testOutput').textContent = 
            `命令: ${data.command}\n\n输出:\n${data.output || data.error || '无输出'}`;
        
        if (res.ok) {
            // 测试成功
            document.getElementById('testResultTitle').textContent = '✅ 测试成功';
            document.getElementById('testResultTitle').style.color = '#10b981';
            document.getElementById('testFixSection').style.display = 'none';
            document.getElementById('testSuccessSection').style.display = 'block';
            showToast('测试成功！可以保存为预设', 'success');
            lastTestError = null;
        } else {
            // 测试失败
            document.getElementById('testResultTitle').textContent = '❌ 测试失败';
            document.getElementById('testResultTitle').style.color = '#ef4444';
            document.getElementById('testFixSection').style.display = 'block';
            document.getElementById('testSuccessSection').style.display = 'none';
            showToast('测试失败，可以让 AI 修正参数', 'error');
            // 保存错误信息用于修正
            lastTestError = {
                error: data.error || '未知错误',
                output: data.output || '',
                command: data.command || ''
            };
        }
    } catch (e) {
        showToast(`测试失败: ${e.message}`, 'error');
        document.getElementById('testResult').style.display = 'block';
        document.getElementById('testResultTitle').textContent = '❌ 测试失败';
        document.getElementById('testResultTitle').style.color = '#ef4444';
        document.getElementById('testOutput').textContent = `错误: ${e.message}`;
        document.getElementById('testFixSection').style.display = 'block';
        document.getElementById('testSuccessSection').style.display = 'none';
        lastTestError = { error: e.message, output: '', command: '' };
    } finally {
        btn.disabled = false;
        btn.textContent = '▶️ 运行测试';
    }
}

// 让 AI 修正失败的参数
async function fixFailedParams() {
    if (!currentAIResult || !lastTestError) {
        showToast('没有可修正的错误信息', 'error');
        return;
    }
    
    const btn = document.getElementById('fixParamsBtn');
    btn.disabled = true;
    btn.textContent = '⏳ AI 分析修正中...';
    
    try {
        const res = await authFetch(`${API_BASE}/llm/fix`, {
            method: 'POST',
            body: JSON.stringify({
                requirement: document.getElementById('aiRequirement').value.trim(),
                input_format: document.getElementById('aiInputFormat').value.trim(),
                failed_args: currentAIResult.ffmpeg_args,
                output_ext: currentAIResult.output_ext,
                error_message: lastTestError.error,
                ffmpeg_output: lastTestError.output
            })
        });
        if (!res) return;
        const data = await res.json();
        
        if (!res.ok) {
            throw new Error(data.error || '修正失败');
        }
        
        // 更新当前结果
        currentAIResult.ffmpeg_args = data.ffmpeg_args;
        currentAIResult.explanation = data.explanation;
        if (data.output_ext) {
            currentAIResult.output_ext = data.output_ext;
        }
        
        // 更新显示
        document.getElementById('currentTestArgs').textContent = data.ffmpeg_args.join(' ');
        document.getElementById('resultArgs').textContent = data.ffmpeg_args.join(' ');
        document.getElementById('resultExplanation').textContent = data.explanation;
        
        // 隐藏修正区域，提示用户重新测试
        document.getElementById('testFixSection').style.display = 'none';
        document.getElementById('testResult').style.display = 'none';
        
        showToast('参数已修正，请重新测试', 'success');
    } catch (e) {
        showToast(`修正失败: ${e.message}`, 'error');
    } finally {
        btn.disabled = false;
        btn.textContent = '🔧 AI 修正参数';
    }
}

// 保存为预设
function saveAsPreset() {
    if (!currentAIResult) {
        showToast('请先生成参数', 'error');
        return;
    }
    document.getElementById('presetName').value = currentAIResult.name;
    document.getElementById('presetDescription').value = currentAIResult.description;
    document.getElementById('savePresetModal').classList.add('active');
}

function closeSavePresetModal() {
    document.getElementById('savePresetModal').classList.remove('active');
}

// 确认保存预设
async function confirmSavePreset() {
    const name = document.getElementById('presetName').value.trim();
    const description = document.getElementById('presetDescription').value.trim();
    
    if (!name) {
        showToast('请输入预设名称', 'error');
        return;
    }
    
    try {
        const res = await authFetch(`${API_BASE}/presets`, {
            method: 'POST',
            body: JSON.stringify({
                name,
                description,
                ffmpeg_args: currentAIResult.ffmpeg_args,
                output_ext: currentAIResult.output_ext
            })
        });
        if (!res) return;
        const data = await res.json();
        
        if (!res.ok) {
            throw new Error(data.error || '保存失败');
        }
        
        showToast(`预设保存成功: ${data.preset_id}`, 'success');
        closeSavePresetModal();
        loadPresets();
        loadTranscodeTypes();
    } catch (e) {
        showToast(`保存失败: ${e.message}`, 'error');
    }
}

// ==================== 预设管理 ====================

// 加载预设列表
async function loadPresets() {
    try {
        const res = await authFetch(`${API_BASE}/presets`);
        if (!res) return;
        const data = await res.json();
        const tbody = document.querySelector('#presetsTable tbody');
        if (!tbody) return;
        tbody.innerHTML = '';
        
        if (data.presets && data.presets.length > 0) {
            data.presets.forEach(preset => {
                const typeClass = preset.is_builtin ? 'builtin' : 'custom';
                const typeText = preset.is_builtin ? '内置' : '自定义';
                tbody.innerHTML += `
                    <tr>
                        <td>${preset.preset_id}</td>
                        <td>${preset.name}</td>
                        <td>${preset.description || '-'}</td>
                        <td>${preset.output_ext}</td>
                        <td><span class="preset-type ${typeClass}">${typeText}</span></td>
                        <td>
                            ${!preset.is_builtin ? `<button class="btn btn-danger btn-small" onclick="deletePreset('${preset.preset_id}')">删除</button>` : '-'}
                        </td>
                    </tr>
                `;
            });
        } else {
            tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#999;">暂无预设</td></tr>';
        }
    } catch (e) {
        console.error('加载预设失败:', e);
    }
}

// 删除预设
async function deletePreset(presetId) {
    if (!confirm('确定要删除此预设吗？')) return;
    
    try {
        const res = await authFetch(`${API_BASE}/presets/${presetId}`, { method: 'DELETE' });
        if (!res) return;
        const data = await res.json();
        
        if (res.ok) {
            showToast('预设删除成功', 'success');
            loadPresets();
            loadTranscodeTypes();
        } else {
            showToast(data.error || '删除失败', 'error');
        }
    } catch (e) {
        showToast('删除预设失败', 'error');
    }
}

// 加载转码类型选项
async function loadTranscodeTypes() {
    try {
        const res = await authFetch(`${API_BASE}/presets`);
        if (!res) return;
        const data = await res.json();
        const container = document.getElementById('transcodeTypeCheckboxes');
        if (!container) return;
        container.innerHTML = '';
        
        if (data.presets && data.presets.length > 0) {
            data.presets.forEach(preset => {
                const checked = ['mp4_standard', 'thumbnail'].includes(preset.preset_id) ? 'checked' : '';
                container.innerHTML += `
                    <label class="checkbox-label">
                        <input type="checkbox" name="transcodeType" value="${preset.preset_id}" ${checked}>
                        ${preset.name} (${preset.preset_id})
                    </label>
                `;
            });
        }
    } catch (e) {
        console.error('加载转码类型失败:', e);
    }
}

// ==================== 仪表盘功能 ====================

async function loadDashboard() {
    await loadTaskStats();
    await loadRecentTasks();
}

async function refreshDashboard() {
    await loadDashboard();
    showToast('统计数据已刷新', 'success');
}

async function loadQueueStats() {
    try {
        const res = await authFetch(`${API_BASE}/queue/status`);
        if (!res) return;
        const data = await res.json();
        document.getElementById('queueWaiting').textContent = data.approximate_number_of_messages || 0;
        document.getElementById('queueProcessing').textContent = data.approximate_number_of_messages_not_visible || 0;
    } catch (e) {
        console.error('加载队列状态失败:', e);
    }
}

async function loadTaskStats() {
    try {
        const queueRes = await authFetch(`${API_BASE}/queue/status`);
        if (!queueRes) return;
        const queueData = await queueRes.json();
        document.getElementById('pendingTasks').textContent = queueData.approximate_number_of_messages || 0;
        document.getElementById('processingTasks').textContent = queueData.approximate_number_of_messages_not_visible || 0;
        
        const completedRes = await authFetch(`${API_BASE}/tasks?status=completed&limit=1`);
        if (!completedRes) return;
        const completedData = await completedRes.json();
        document.getElementById('completedTasks').textContent = completedData.total || 0;
        
        const failedRes = await authFetch(`${API_BASE}/tasks?status=failed&limit=1`);
        if (!failedRes) return;
        const failedData = await failedRes.json();
        document.getElementById('failedTasks').textContent = failedData.total || 0;
    } catch (e) {
        console.error('加载任务统计失败:', e);
    }
}

async function showTasksByStatus(status) {
    dashboardTasksStatus = status;
    dashboardTasksPage = 1;
    await loadDashboardTasks();
    document.getElementById('dashboardTasksSection').style.display = 'block';
    const statusNames = { 'pending': '等待中', 'processing': '处理中', 'completed': '已完成', 'failed': '失败' };
    document.getElementById('dashboardTasksTitle').textContent = `📋 ${statusNames[status] || status}任务`;
    document.getElementById('dashboardTasksSection').scrollIntoView({ behavior: 'smooth' });
}

async function loadDashboardTasks() {
    const offset = (dashboardTasksPage - 1) * pageSize;
    try {
        const res = await authFetch(`${API_BASE}/tasks?status=${dashboardTasksStatus}&limit=${pageSize}&offset=${offset}`);
        if (!res) return;
        const data = await res.json();
        dashboardTasksTotal = data.total || 0;
        const tbody = document.querySelector('#dashboardTasksTable tbody');
        tbody.innerHTML = '';
        if (data.tasks && data.tasks.length > 0) {
            data.tasks.forEach(task => { tbody.innerHTML += createTaskRow(task, false); });
        } else {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#999;">暂无任务</td></tr>';
        }
        renderDashboardPagination();
    } catch (e) {
        console.error('加载仪表盘任务列表失败:', e);
        showToast('加载任务列表失败', 'error');
    }
}

function renderDashboardPagination() {
    const totalPages = Math.ceil(dashboardTasksTotal / pageSize);
    const pagination = document.getElementById('dashboardTasksPagination');
    if (totalPages <= 1) {
        pagination.innerHTML = dashboardTasksTotal > 0 ? `<span style="color:#666;">共 ${dashboardTasksTotal} 条</span>` : '';
        return;
    }
    let html = `<button ${dashboardTasksPage === 1 ? 'disabled' : ''} onclick="goToDashboardPage(${dashboardTasksPage - 1})">上一页</button>`;
    const startPage = Math.max(1, dashboardTasksPage - 2);
    const endPage = Math.min(totalPages, startPage + 4);
    for (let i = startPage; i <= endPage; i++) {
        html += `<button class="${i === dashboardTasksPage ? 'active' : ''}" onclick="goToDashboardPage(${i})">${i}</button>`;
    }
    html += `<button ${dashboardTasksPage === totalPages ? 'disabled' : ''} onclick="goToDashboardPage(${dashboardTasksPage + 1})">下一页</button>`;
    html += `<span style="margin-left:10px;color:#666;">共 ${dashboardTasksTotal} 条</span>`;
    pagination.innerHTML = html;
}

function goToDashboardPage(page) { dashboardTasksPage = page; loadDashboardTasks(); }
function closeDashboardTasks() { document.getElementById('dashboardTasksSection').style.display = 'none'; }

async function loadRecentTasks() {
    try {
        const res = await authFetch(`${API_BASE}/tasks?limit=5`);
        if (!res) return;
        const data = await res.json();
        const tbody = document.querySelector('#recentTasksTable tbody');
        tbody.innerHTML = '';
        if (data.tasks && data.tasks.length > 0) {
            data.tasks.forEach(task => { tbody.innerHTML += createTaskRow(task, true); });
        } else {
            tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;color:#999;">暂无任务</td></tr>';
        }
    } catch (e) {
        console.error('加载最近任务失败:', e);
    }
}

// ==================== 任务管理功能 ====================

async function loadTasks() {
    const status = document.getElementById('statusFilter').value;
    const date = document.getElementById('dateFilter').value;
    const offset = (currentPage - 1) * pageSize;
    let url = `${API_BASE}/tasks?limit=${pageSize}&offset=${offset}`;
    if (status) url += `&status=${status}`;
    if (date) url += `&date=${date}`;
    
    try {
        const res = await authFetch(url);
        if (!res) return;
        const data = await res.json();
        totalTasks = data.total || 0;
        const tbody = document.querySelector('#tasksTable tbody');
        tbody.innerHTML = '';
        if (data.tasks && data.tasks.length > 0) {
            data.tasks.forEach(task => { tbody.innerHTML += createTaskRow(task, false); });
        } else {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#999;">暂无任务</td></tr>';
        }
        renderPagination();
    } catch (e) {
        console.error('加载任务列表失败:', e);
        showToast('加载任务列表失败', 'error');
    }
}

function refreshTasks() { currentPage = 1; loadTasks(); loadQueueStats(); showToast('任务列表已刷新', 'success'); }
function clearDateFilter() { document.getElementById('dateFilter').value = ''; currentPage = 1; loadTasks(); }

function createTaskRow(task, simple) {
    const statusClass = `status-${task.status}`;
    const statusText = getStatusText(task.status);
    const createdAt = formatDate(task.created_at);
    const shortId = task.task_id.substring(0, 8) + '...';
    
    if (simple) {
        const canRerunSimple = task.status !== 'processing';
        const canAbortSimple = task.status === 'processing';
        return `<tr>
            <td title="${task.task_id}">${shortId}</td>
            <td title="${task.input_key}">${truncate(task.input_key, 30)}</td>
            <td><span class="status-badge ${statusClass}">${statusText}</span></td>
            <td>${createdAt}</td>
            <td><div class="action-btns">
                <button class="btn btn-secondary btn-small" onclick="viewTask('${task.task_id}')">详情</button>
                ${canRerunSimple ? `<button class="btn btn-primary btn-small" onclick="retryTask('${task.task_id}')">重新运行</button>` : ''}
                ${canAbortSimple ? `<button class="btn btn-danger btn-small" onclick="abortTask('${task.task_id}')">中止</button>` : ''}
            </div></td>
        </tr>`;
    }
    
    const transcodeTypes = task.transcode_types ? task.transcode_types.join(', ') : '-';
    const progress = getProgressSummary(task.progress);
    const canRerun = task.status !== 'processing';
    const canCancel = task.status === 'pending';
    const canAbort = task.status === 'processing';
    
    return `<tr>
        <td title="${task.task_id}">${shortId}</td>
        <td title="${task.input_key}">${truncate(task.input_key, 25)}</td>
        <td title="${transcodeTypes}">${truncate(transcodeTypes, 20)}</td>
        <td><span class="status-badge ${statusClass}">${statusText}</span></td>
        <td>${progress}</td>
        <td>${createdAt}</td>
        <td><div class="action-btns">
            <button class="btn btn-secondary btn-small" onclick="viewTask('${task.task_id}')">详情</button>
            ${canRerun ? `<button class="btn btn-primary btn-small" onclick="retryTask('${task.task_id}')">重新运行</button>` : ''}
            ${canCancel ? `<button class="btn btn-danger btn-small" onclick="cancelTask('${task.task_id}')">取消</button>` : ''}
            ${canAbort ? `<button class="btn btn-danger btn-small" onclick="abortTask('${task.task_id}')">中止</button>` : ''}
        </div></td>
    </tr>`;
}

function getProgressSummary(progress) {
    if (!progress) return '-';
    const values = Object.values(progress);
    const completed = values.filter(v => v === 'completed').length;
    return values.length === 0 ? '-' : `${completed}/${values.length}`;
}

function renderPagination() {
    const totalPages = Math.ceil(totalTasks / pageSize);
    const pagination = document.getElementById('tasksPagination');
    if (totalPages <= 1) {
        pagination.innerHTML = totalTasks > 0 ? `<span style="color:#666;">共 ${totalTasks} 条</span>` : '';
        return;
    }
    let html = `<button ${currentPage === 1 ? 'disabled' : ''} onclick="goToPage(${currentPage - 1})">上一页</button>`;
    const startPage = Math.max(1, currentPage - 2);
    const endPage = Math.min(totalPages, startPage + 4);
    if (startPage > 1) {
        html += `<button onclick="goToPage(1)">1</button>`;
        if (startPage > 2) html += `<span style="padding:0 8px;">...</span>`;
    }
    for (let i = startPage; i <= endPage; i++) {
        html += `<button class="${i === currentPage ? 'active' : ''}" onclick="goToPage(${i})">${i}</button>`;
    }
    if (endPage < totalPages) {
        if (endPage < totalPages - 1) html += `<span style="padding:0 8px;">...</span>`;
        html += `<button onclick="goToPage(${totalPages})">${totalPages}</button>`;
    }
    html += `<button ${currentPage === totalPages ? 'disabled' : ''} onclick="goToPage(${currentPage + 1})">下一页</button>`;
    html += `<span style="margin-left:10px;color:#666;">共 ${totalTasks} 条</span>`;
    pagination.innerHTML = html;
}

function goToPage(page) { currentPage = page; loadTasks(); }

// ==================== 任务详情和操作 ====================

async function viewTask(taskId) {
    try {
        const res = await authFetch(`${API_BASE}/tasks/${taskId}`);
        if (!res) return;
        const task = await res.json();
        document.getElementById('taskDetailContent').innerHTML = createTaskDetail(task);
        document.getElementById('taskDetailModal').classList.add('active');
    } catch (e) {
        showToast('获取任务详情失败', 'error');
    }
}

function createTaskDetail(task) {
    const statusClass = `status-${task.status}`;
    const statusText = getStatusText(task.status);
    let html = `<div class="detail-grid">
        <div class="detail-item"><div class="detail-label">任务ID</div><div class="detail-value">${task.task_id}</div></div>
        <div class="detail-item"><div class="detail-label">状态</div><div class="detail-value"><span class="status-badge ${statusClass}">${statusText}</span></div></div>
        <div class="detail-item"><div class="detail-label">输入桶</div><div class="detail-value">${task.input_bucket}</div></div>
        <div class="detail-item"><div class="detail-label">输入文件</div><div class="detail-value">${task.input_key}</div></div>
        <div class="detail-item"><div class="detail-label">输出桶</div><div class="detail-value">${task.output_bucket || '-'}</div></div>
        <div class="detail-item"><div class="detail-label">重试次数</div><div class="detail-value">${task.retry_count} / ${task.max_retries}</div></div>
        <div class="detail-item"><div class="detail-label">创建时间</div><div class="detail-value">${formatDate(task.created_at)}</div></div>
        <div class="detail-item"><div class="detail-label">更新时间</div><div class="detail-value">${formatDate(task.updated_at)}</div></div>
    </div>`;
    
    if (task.progress && Object.keys(task.progress).length > 0) {
        html += `<h4 style="margin-top:20px;margin-bottom:10px;">转码进度</h4><div class="progress-list">`;
        for (const [type, status] of Object.entries(task.progress)) {
            const progressClass = status === 'completed' ? 'status-completed' : status === 'failed' ? 'status-failed' : 'status-pending';
            html += `<div class="progress-item"><span>${type}</span><span class="status-badge ${progressClass}">${status}</span></div>`;
        }
        html += `</div>`;
    }
    
    if (task.output_files && Object.keys(task.output_files).length > 0) {
        html += `<h4 style="margin-top:20px;margin-bottom:10px;">输出文件</h4><div class="progress-list">`;
        for (const [type, path] of Object.entries(task.output_files)) {
            html += `<div class="progress-item"><span>${type}</span><span style="word-break:break-all;">${path}</span></div>`;
        }
        html += `</div>`;
    }
    
    if (task.error_message) {
        html += `<div class="error-box"><h4>❌ 错误信息</h4><p>${task.error_message}</p></div>`;
    }
    
    if (task.error_details && task.error_details.length > 0) {
        html += `<h4 style="margin-top:20px;margin-bottom:10px;">错误详情</h4>`;
        task.error_details.forEach((detail, index) => {
            html += `<div class="error-box" style="margin-top:10px;">
                <h4>错误 ${index + 1}: ${detail.transcode_type} - ${detail.stage}</h4>
                <p><strong>错误:</strong> ${detail.error}</p>
                ${detail.command ? `<p><strong>命令:</strong> <code style="word-break:break-all;">${detail.command}</code></p>` : ''}
                ${detail.output ? `<pre style="background:#f3f4f6;padding:10px;border-radius:4px;overflow-x:auto;font-size:12px;max-height:200px;">${escapeHtml(detail.output)}</pre>` : ''}
            </div>`;
        });
    }
    
    const canRerun = task.status !== 'processing';
    const canCancel = task.status === 'pending';
    const canAbort = task.status === 'processing';
    html += `<div style="margin-top:24px;display:flex;gap:12px;">
        ${canRerun ? `<button class="btn btn-primary" onclick="retryTask('${task.task_id}');closeModal();">🔄 重新运行</button>` : ''}
        ${canCancel ? `<button class="btn btn-danger" onclick="cancelTask('${task.task_id}');closeModal();">❌ 取消任务</button>` : ''}
        ${canAbort ? `<button class="btn btn-danger" onclick="abortTask('${task.task_id}');closeModal();">⛔ 中止任务</button>` : ''}
        <button class="btn btn-secondary" onclick="closeModal()">关闭</button>
    </div>`;
    return html;
}

function closeModal() { document.getElementById('taskDetailModal').classList.remove('active'); }

async function retryTask(taskId) {
    if (!confirm('确定要重新运行此任务吗？')) return;
    try {
        const res = await authFetch(`${API_BASE}/tasks/${taskId}/retry`, { method: 'POST' });
        if (!res) return;
        const data = await res.json();
        if (res.ok) { showToast('任务已重新加入队列', 'success'); loadTasks(); loadDashboard(); }
        else { showToast(data.error || '重新运行失败', 'error'); }
    } catch (e) { showToast('重新运行任务失败', 'error'); }
}

async function cancelTask(taskId) {
    if (!confirm('确定要取消此任务吗？')) return;
    try {
        const res = await authFetch(`${API_BASE}/tasks/${taskId}`, { method: 'DELETE' });
        if (!res) return;
        const data = await res.json();
        if (res.ok) { showToast('任务已取消', 'success'); loadTasks(); loadDashboard(); }
        else { showToast(data.error || '取消失败', 'error'); }
    } catch (e) { showToast('取消任务失败', 'error'); }
}

async function abortTask(taskId) {
    if (!confirm('⚠️ 确定要中止此正在运行的任务吗？')) return;
    try {
        const res = await authFetch(`${API_BASE}/tasks/${taskId}/abort`, { method: 'POST' });
        if (!res) return;
        const data = await res.json();
        if (res.ok) { showToast('任务已中止', 'success'); loadTasks(); loadDashboard(); }
        else { showToast(data.error || '中止失败', 'error'); }
    } catch (e) { showToast('中止任务失败', 'error'); }
}

// ==================== 队列管理 ====================

async function refreshQueueStatus() { await loadQueueStats(); showToast('队列状态已刷新', 'success'); }

async function purgeQueue() {
    if (!confirm('⚠️ 确定要清空队列吗？此操作不可恢复！')) return;
    try {
        const res = await authFetch(`${API_BASE}/queue/purge`, { method: 'DELETE' });
        if (!res) return;
        const data = await res.json();
        if (res.ok) { showToast('队列已清空', 'success'); loadQueueStats(); }
        else { showToast(data.error || '清空队列失败', 'error'); }
    } catch (e) { showToast('清空队列失败', 'error'); }
}

// ==================== 提交任务 ====================

async function submitTask(event) {
    event.preventDefault();
    const inputBucket = document.getElementById('inputBucket').value.trim();
    const inputKey = document.getElementById('inputKey').value.trim();
    const checkboxes = document.querySelectorAll('input[name="transcodeType"]:checked');
    
    if (checkboxes.length === 0) { showToast('请至少选择一种转码类型', 'error'); return; }
    
    const transcodeTypes = Array.from(checkboxes).map(cb => cb.value);
    try {
        const res = await authFetch(`${API_BASE}/queue/add`, {
            method: 'POST',
            body: JSON.stringify({ input_bucket: inputBucket, input_key: inputKey, transcode_types: transcodeTypes })
        });
        if (!res) return;
        const data = await res.json();
        if (res.ok) {
            showToast(`任务创建成功: ${data.task_id}`, 'success');
            document.getElementById('addTaskForm').reset();
            loadDashboard();
        } else { showToast(data.error || '创建任务失败', 'error'); }
    } catch (e) { showToast('创建任务失败', 'error'); }
}

// ==================== 工具函数 ====================

function getStatusText(status) {
    const map = { 'pending': '等待中', 'processing': '处理中', 'completed': '已完成', 'failed': '失败', 'retrying': '重试中', 'cancelled': '已取消' };
    return map[status] || status;
}

function formatDate(dateStr) {
    if (!dateStr) return '-';
    const date = new Date(dateStr);
    return date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function truncate(str, len) { return !str ? '-' : str.length > len ? str.substring(0, len) + '...' : str; }

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function showToast(message, type = 'info') {
    const container = document.getElementById('toastContainer');
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.innerHTML = `<span>${type === 'success' ? '✅' : type === 'error' ? '❌' : 'ℹ️'}</span> ${message}`;
    container.appendChild(toast);
    setTimeout(() => { toast.remove(); }, 3000);
}

// 点击模态框外部关闭
document.getElementById('taskDetailModal')?.addEventListener('click', (e) => { if (e.target.id === 'taskDetailModal') closeModal(); });
document.getElementById('testModal')?.addEventListener('click', (e) => { if (e.target.id === 'testModal') closeTestModal(); });
document.getElementById('savePresetModal')?.addEventListener('click', (e) => { if (e.target.id === 'savePresetModal') closeSavePresetModal(); });

// ==================== 表格列宽拖拽调整功能 ====================

function initTableResize() {
    const observer = new MutationObserver(() => {
        document.querySelectorAll('.data-table').forEach(table => {
            if (!table.dataset.resizeInit) { setupTableResize(table); table.dataset.resizeInit = 'true'; }
        });
    });
    observer.observe(document.body, { childList: true, subtree: true });
    document.querySelectorAll('.data-table').forEach(table => { setupTableResize(table); table.dataset.resizeInit = 'true'; });
}

function setupTableResize(table) {
    const headerCells = table.querySelectorAll('th');
    headerCells.forEach((th, index) => {
        if (index === headerCells.length - 1) return;
        if (th.querySelector('.resize-handle')) return;
        const handle = document.createElement('div');
        handle.className = 'resize-handle';
        th.appendChild(handle);
        let startX, startWidth;
        handle.addEventListener('mousedown', (e) => {
            e.preventDefault(); e.stopPropagation();
            startX = e.pageX; startWidth = th.offsetWidth;
            handle.classList.add('resizing'); table.classList.add('resizing');
            document.addEventListener('mousemove', onMouseMove);
            document.addEventListener('mouseup', onMouseUp);
        });
        function onMouseMove(e) {
            const diff = e.pageX - startX;
            const newWidth = Math.max(80, startWidth + diff);
            th.style.width = newWidth + 'px'; th.classList.add('resized');
            table.querySelectorAll('tbody tr').forEach(row => {
                const cells = row.querySelectorAll('td');
                if (cells[index]) { cells[index].style.width = newWidth + 'px'; cells[index].classList.add('resized'); }
            });
        }
        function onMouseUp() {
            handle.classList.remove('resizing'); table.classList.remove('resizing');
            document.removeEventListener('mousemove', onMouseMove);
            document.removeEventListener('mouseup', onMouseUp);
        }
    });
}


// ==================== 用户管理功能 ====================

// 加载用户列表
async function loadUsers() {
    try {
        const res = await authFetch(`${API_BASE}/users`);
        if (!res) return;
        const data = await res.json();
        const tbody = document.querySelector('#usersTable tbody');
        if (!tbody) return;
        tbody.innerHTML = '';
        
        if (data.users && data.users.length > 0) {
            data.users.forEach(user => {
                const roleClass = user.role === 'admin' ? 'admin' : 'user';
                const roleText = user.role === 'admin' ? '管理员' : '普通用户';
                const isAdmin = user.username === 'admin';
                tbody.innerHTML += `
                    <tr>
                        <td>${user.username}</td>
                        <td><span class="role-badge ${roleClass}">${roleText}</span></td>
                        <td>${formatDate(user.created_at)}</td>
                        <td>
                            <div class="action-btns">
                                <button class="btn btn-secondary btn-small" onclick="showChangePasswordModal('${user.username}')">修改密码</button>
                                ${!isAdmin ? `<button class="btn btn-danger btn-small" onclick="deleteUser('${user.username}')">删除</button>` : ''}
                            </div>
                        </td>
                    </tr>
                `;
            });
        } else {
            tbody.innerHTML = '<tr><td colspan="4" style="text-align:center;color:#999;">暂无用户</td></tr>';
        }
    } catch (e) {
        console.error('加载用户列表失败:', e);
        showToast('加载用户列表失败', 'error');
    }
}

// 显示添加用户模态框
function showAddUserModal() {
    document.getElementById('newUsername').value = '';
    document.getElementById('newPassword').value = '';
    document.getElementById('newRole').value = 'user';
    document.getElementById('addUserModal').classList.add('active');
}

// 关闭添加用户模态框
function closeAddUserModal() {
    document.getElementById('addUserModal').classList.remove('active');
}

// 创建用户
async function createUser() {
    const username = document.getElementById('newUsername').value.trim();
    const password = document.getElementById('newPassword').value;
    const role = document.getElementById('newRole').value;
    
    if (!username || !password) {
        showToast('请填写用户名和密码', 'error');
        return;
    }
    
    try {
        const res = await authFetch(`${API_BASE}/users`, {
            method: 'POST',
            body: JSON.stringify({ username, password, role })
        });
        if (!res) return;
        const data = await res.json();
        
        if (res.ok) {
            showToast('用户创建成功', 'success');
            closeAddUserModal();
            loadUsers();
        } else {
            showToast(data.error || '创建用户失败', 'error');
        }
    } catch (e) {
        showToast('创建用户失败', 'error');
    }
}

// 删除用户
async function deleteUser(username) {
    if (!confirm(`确定要删除用户 "${username}" 吗？`)) return;
    
    try {
        const res = await authFetch(`${API_BASE}/users/${username}`, { method: 'DELETE' });
        if (!res) return;
        const data = await res.json();
        
        if (res.ok) {
            showToast('用户删除成功', 'success');
            loadUsers();
        } else {
            showToast(data.error || '删除用户失败', 'error');
        }
    } catch (e) {
        showToast('删除用户失败', 'error');
    }
}

// 显示修改密码模态框
function showChangePasswordModal(username) {
    document.getElementById('changePasswordUsername').value = username;
    document.getElementById('changeNewPassword').value = '';
    document.getElementById('changePasswordModal').classList.add('active');
}

// 关闭修改密码模态框
function closeChangePasswordModal() {
    document.getElementById('changePasswordModal').classList.remove('active');
}

// 修改用户密码
async function changeUserPassword() {
    const username = document.getElementById('changePasswordUsername').value;
    const newPassword = document.getElementById('changeNewPassword').value;
    
    if (!newPassword) {
        showToast('请输入新密码', 'error');
        return;
    }
    
    try {
        const res = await authFetch(`${API_BASE}/users/${username}/password`, {
            method: 'PUT',
            body: JSON.stringify({ new_password: newPassword })
        });
        if (!res) return;
        const data = await res.json();
        
        if (res.ok) {
            showToast('密码修改成功', 'success');
            closeChangePasswordModal();
        } else {
            showToast(data.error || '修改密码失败', 'error');
        }
    } catch (e) {
        showToast('修改密码失败', 'error');
    }
}

// 点击模态框外部关闭
document.getElementById('addUserModal')?.addEventListener('click', (e) => { if (e.target.id === 'addUserModal') closeAddUserModal(); });
document.getElementById('changePasswordModal')?.addEventListener('click', (e) => { if (e.target.id === 'changePasswordModal') closeChangePasswordModal(); });
