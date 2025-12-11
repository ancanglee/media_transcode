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

// 初始化
document.addEventListener('DOMContentLoaded', () => {
    initTabs();
    initDateFilter();
    checkHealth();
    loadDashboard();
    initTableResize();
    loadSystemConfig();
});

// 加载系统配置
async function loadSystemConfig() {
    try {
        const res = await fetch(`${API_BASE}/config`);
        const data = await res.json();
        
        systemConfig.inputBucket = data.input_bucket || '';
        systemConfig.outputBucket = data.output_bucket || '';
        
        // 填充输入桶默认值
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
            
            // 切换到对应 tab 时加载数据
            if (tab.dataset.tab === 'task-queue') {
                loadTasks();
                loadQueueStats();
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
    
    // 绑定筛选器变化事件，自动触发查询
    dateFilter.addEventListener('change', () => {
        currentPage = 1;
        loadTasks();
    });
    
    statusFilter.addEventListener('change', () => {
        currentPage = 1;
        loadTasks();
    });
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

// 加载仪表盘数据
async function loadDashboard() {
    await loadTaskStats();
    await loadRecentTasks();
}

// 手动刷新仪表盘
async function refreshDashboard() {
    await loadDashboard();
    showToast('统计数据已刷新', 'success');
}

// 加载队列统计
async function loadQueueStats() {
    try {
        const res = await fetch(`${API_BASE}/queue/status`);
        const data = await res.json();
        
        document.getElementById('queueWaiting').textContent = data.approximate_number_of_messages || 0;
        document.getElementById('queueProcessing').textContent = data.approximate_number_of_messages_not_visible || 0;
    } catch (e) {
        console.error('加载队列状态失败:', e);
    }
}

// 加载任务统计
async function loadTaskStats() {
    console.log('📊 loadTaskStats v2 - 开始加载统计数据');
    try {
        // 从 SQS 获取队列状态（等待中和处理中）
        console.log('📊 请求 SQS 队列状态...');
        const queueRes = await fetch(`${API_BASE}/queue/status`);
        const queueData = await queueRes.json();
        console.log('📊 SQS 队列状态:', queueData);
        document.getElementById('pendingTasks').textContent = queueData.approximate_number_of_messages || 0;
        document.getElementById('processingTasks').textContent = queueData.approximate_number_of_messages_not_visible || 0;
        
        // 从 DynamoDB 获取已完成任务
        console.log('📊 请求已完成任务统计...');
        const completedRes = await fetch(`${API_BASE}/tasks?status=completed&limit=1`);
        const completedData = await completedRes.json();
        console.log('📊 已完成任务:', completedData);
        document.getElementById('completedTasks').textContent = completedData.total || 0;
        
        // 从 DynamoDB 获取失败任务
        console.log('📊 请求失败任务统计...');
        const failedRes = await fetch(`${API_BASE}/tasks?status=failed&limit=1`);
        const failedData = await failedRes.json();
        console.log('📊 失败任务:', failedData);
        document.getElementById('failedTasks').textContent = failedData.total || 0;
        
        console.log('📊 loadTaskStats v2 - 统计数据加载完成');
    } catch (e) {
        console.error('加载任务统计失败:', e);
    }
}

// 点击统计卡片展示对应状态的任务列表
async function showTasksByStatus(status) {
    dashboardTasksStatus = status;
    dashboardTasksPage = 1;
    await loadDashboardTasks();
    
    // 显示任务列表区域
    document.getElementById('dashboardTasksSection').style.display = 'block';
    
    // 更新标题
    const statusNames = {
        'pending': '等待中',
        'processing': '处理中',
        'completed': '已完成',
        'failed': '失败'
    };
    document.getElementById('dashboardTasksTitle').textContent = `📋 ${statusNames[status] || status}任务`;
    
    // 滚动到任务列表
    document.getElementById('dashboardTasksSection').scrollIntoView({ behavior: 'smooth' });
}

// 加载仪表盘任务列表
async function loadDashboardTasks() {
    const offset = (dashboardTasksPage - 1) * pageSize;
    
    try {
        const res = await fetch(`${API_BASE}/tasks?status=${dashboardTasksStatus}&limit=${pageSize}&offset=${offset}`);
        const data = await res.json();
        
        dashboardTasksTotal = data.total || 0;
        
        const tbody = document.querySelector('#dashboardTasksTable tbody');
        tbody.innerHTML = '';
        
        if (data.tasks && data.tasks.length > 0) {
            data.tasks.forEach(task => {
                tbody.innerHTML += createTaskRow(task, false);
            });
        } else {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#999;">暂无任务</td></tr>';
        }
        
        renderDashboardPagination();
    } catch (e) {
        console.error('加载仪表盘任务列表失败:', e);
        showToast('加载任务列表失败', 'error');
    }
}

// 渲染仪表盘任务列表分页
function renderDashboardPagination() {
    const totalPages = Math.ceil(dashboardTasksTotal / pageSize);
    const pagination = document.getElementById('dashboardTasksPagination');
    
    if (totalPages <= 1) {
        pagination.innerHTML = dashboardTasksTotal > 0 ? `<span style="color:#666;">共 ${dashboardTasksTotal} 条</span>` : '';
        return;
    }
    
    let html = '';
    html += `<button ${dashboardTasksPage === 1 ? 'disabled' : ''} onclick="goToDashboardPage(${dashboardTasksPage - 1})">上一页</button>`;
    
    // 显示页码
    const startPage = Math.max(1, dashboardTasksPage - 2);
    const endPage = Math.min(totalPages, startPage + 4);
    
    for (let i = startPage; i <= endPage; i++) {
        html += `<button class="${i === dashboardTasksPage ? 'active' : ''}" onclick="goToDashboardPage(${i})">${i}</button>`;
    }
    
    html += `<button ${dashboardTasksPage === totalPages ? 'disabled' : ''} onclick="goToDashboardPage(${dashboardTasksPage + 1})">下一页</button>`;
    html += `<span style="margin-left:10px;color:#666;">共 ${dashboardTasksTotal} 条</span>`;
    
    pagination.innerHTML = html;
}

// 仪表盘任务列表翻页
function goToDashboardPage(page) {
    dashboardTasksPage = page;
    loadDashboardTasks();
}

// 关闭仪表盘任务列表
function closeDashboardTasks() {
    document.getElementById('dashboardTasksSection').style.display = 'none';
}

// 加载最近任务
async function loadRecentTasks() {
    try {
        const res = await fetch(`${API_BASE}/tasks?limit=5`);
        const data = await res.json();
        
        const tbody = document.querySelector('#recentTasksTable tbody');
        tbody.innerHTML = '';
        
        if (data.tasks && data.tasks.length > 0) {
            data.tasks.forEach(task => {
                tbody.innerHTML += createTaskRow(task, true);
            });
        } else {
            tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;color:#999;">暂无任务</td></tr>';
        }
    } catch (e) {
        console.error('加载最近任务失败:', e);
    }
}

// 加载任务列表
async function loadTasks() {
    const status = document.getElementById('statusFilter').value;
    const date = document.getElementById('dateFilter').value;
    const offset = (currentPage - 1) * pageSize;
    
    let url = `${API_BASE}/tasks?limit=${pageSize}&offset=${offset}`;
    if (status) url += `&status=${status}`;
    if (date) url += `&date=${date}`;
    
    try {
        const res = await fetch(url);
        const data = await res.json();
        
        totalTasks = data.total || 0;
        
        const tbody = document.querySelector('#tasksTable tbody');
        tbody.innerHTML = '';
        
        if (data.tasks && data.tasks.length > 0) {
            data.tasks.forEach(task => {
                tbody.innerHTML += createTaskRow(task, false);
            });
        } else {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#999;">暂无任务</td></tr>';
        }
        
        renderPagination();
    } catch (e) {
        console.error('加载任务列表失败:', e);
        showToast('加载任务列表失败', 'error');
    }
}

// 刷新任务列表
function refreshTasks() {
    currentPage = 1;
    loadTasks();
    loadQueueStats();
    showToast('任务列表已刷新', 'success');
}

// 清除日期筛选
function clearDateFilter() {
    document.getElementById('dateFilter').value = '';
    currentPage = 1;
    loadTasks();
}

// 创建任务行
function createTaskRow(task, simple) {
    const statusClass = `status-${task.status}`;
    const statusText = getStatusText(task.status);
    const createdAt = formatDate(task.created_at);
    const shortId = task.task_id.substring(0, 8) + '...';
    
    if (simple) {
        const canRerunSimple = task.status !== 'processing';
        const canAbortSimple = task.status === 'processing';
        return `
            <tr>
                <td title="${task.task_id}">${shortId}</td>
                <td title="${task.input_key}">${truncate(task.input_key, 30)}</td>
                <td><span class="status-badge ${statusClass}">${statusText}</span></td>
                <td>${createdAt}</td>
                <td>
                    <div class="action-btns">
                        <button class="btn btn-secondary btn-small" onclick="viewTask('${task.task_id}')">详情</button>
                        ${canRerunSimple ? `<button class="btn btn-primary btn-small" onclick="retryTask('${task.task_id}')">重新运行</button>` : ''}
                        ${canAbortSimple ? `<button class="btn btn-danger btn-small" onclick="abortTask('${task.task_id}')">中止</button>` : ''}
                    </div>
                </td>
            </tr>
        `;
    }
    
    const transcodeTypes = task.transcode_types ? task.transcode_types.join(', ') : '-';
    const progress = getProgressSummary(task.progress);
    
    // 除了 processing 状态，其他状态都可以重新运行
    const canRerun = task.status !== 'processing';
    const canCancel = task.status === 'pending';
    const canAbort = task.status === 'processing';
    
    return `
        <tr>
            <td title="${task.task_id}">${shortId}</td>
            <td title="${task.input_key}">${truncate(task.input_key, 25)}</td>
            <td title="${transcodeTypes}">${truncate(transcodeTypes, 20)}</td>
            <td><span class="status-badge ${statusClass}">${statusText}</span></td>
            <td>${progress}</td>
            <td>${createdAt}</td>
            <td>
                <div class="action-btns">
                    <button class="btn btn-secondary btn-small" onclick="viewTask('${task.task_id}')">详情</button>
                    ${canRerun ? `<button class="btn btn-primary btn-small" onclick="retryTask('${task.task_id}')">重新运行</button>` : ''}
                    ${canCancel ? `<button class="btn btn-danger btn-small" onclick="cancelTask('${task.task_id}')">取消</button>` : ''}
                    ${canAbort ? `<button class="btn btn-danger btn-small" onclick="abortTask('${task.task_id}')">中止</button>` : ''}
                </div>
            </td>
        </tr>
    `;
}

// 获取进度摘要
function getProgressSummary(progress) {
    if (!progress) return '-';
    
    const values = Object.values(progress);
    const completed = values.filter(v => v === 'completed').length;
    const total = values.length;
    
    if (total === 0) return '-';
    return `${completed}/${total}`;
}

// 渲染分页
function renderPagination() {
    const totalPages = Math.ceil(totalTasks / pageSize);
    const pagination = document.getElementById('tasksPagination');
    
    if (totalPages <= 1) {
        pagination.innerHTML = totalTasks > 0 ? `<span style="color:#666;">共 ${totalTasks} 条</span>` : '';
        return;
    }
    
    let html = '';
    html += `<button ${currentPage === 1 ? 'disabled' : ''} onclick="goToPage(${currentPage - 1})">上一页</button>`;
    
    // 显示页码，当前页前后各显示2页
    const startPage = Math.max(1, currentPage - 2);
    const endPage = Math.min(totalPages, startPage + 4);
    
    if (startPage > 1) {
        html += `<button onclick="goToPage(1)">1</button>`;
        if (startPage > 2) {
            html += `<span style="padding:0 8px;">...</span>`;
        }
    }
    
    for (let i = startPage; i <= endPage; i++) {
        html += `<button class="${i === currentPage ? 'active' : ''}" onclick="goToPage(${i})">${i}</button>`;
    }
    
    if (endPage < totalPages) {
        if (endPage < totalPages - 1) {
            html += `<span style="padding:0 8px;">...</span>`;
        }
        html += `<button onclick="goToPage(${totalPages})">${totalPages}</button>`;
    }
    
    html += `<button ${currentPage === totalPages ? 'disabled' : ''} onclick="goToPage(${currentPage + 1})">下一页</button>`;
    html += `<span style="margin-left:10px;color:#666;">共 ${totalTasks} 条</span>`;
    
    pagination.innerHTML = html;
}

// 跳转页面
function goToPage(page) {
    currentPage = page;
    loadTasks();
}

// 查看任务详情
async function viewTask(taskId) {
    try {
        const res = await fetch(`${API_BASE}/tasks/${taskId}`);
        const task = await res.json();
        
        const content = document.getElementById('taskDetailContent');
        content.innerHTML = createTaskDetail(task);
        
        document.getElementById('taskDetailModal').classList.add('active');
    } catch (e) {
        showToast('获取任务详情失败', 'error');
    }
}

// 创建任务详情内容
function createTaskDetail(task) {
    const statusClass = `status-${task.status}`;
    const statusText = getStatusText(task.status);
    
    let html = `
        <div class="detail-grid">
            <div class="detail-item">
                <div class="detail-label">任务ID</div>
                <div class="detail-value">${task.task_id}</div>
            </div>
            <div class="detail-item">
                <div class="detail-label">状态</div>
                <div class="detail-value"><span class="status-badge ${statusClass}">${statusText}</span></div>
            </div>
            <div class="detail-item">
                <div class="detail-label">输入桶</div>
                <div class="detail-value">${task.input_bucket}</div>
            </div>
            <div class="detail-item">
                <div class="detail-label">输入文件</div>
                <div class="detail-value">${task.input_key}</div>
            </div>
            <div class="detail-item">
                <div class="detail-label">输出桶</div>
                <div class="detail-value">${task.output_bucket || '-'}</div>
            </div>
            <div class="detail-item">
                <div class="detail-label">重试次数</div>
                <div class="detail-value">${task.retry_count} / ${task.max_retries}</div>
            </div>
            <div class="detail-item">
                <div class="detail-label">创建时间</div>
                <div class="detail-value">${formatDate(task.created_at)}</div>
            </div>
            <div class="detail-item">
                <div class="detail-label">更新时间</div>
                <div class="detail-value">${formatDate(task.updated_at)}</div>
            </div>
        </div>
    `;
    
    // 转码进度
    if (task.progress && Object.keys(task.progress).length > 0) {
        html += `<h4 style="margin-top:20px;margin-bottom:10px;">转码进度</h4><div class="progress-list">`;
        for (const [type, status] of Object.entries(task.progress)) {
            const progressClass = status === 'completed' ? 'status-completed' : 
                                  status === 'failed' ? 'status-failed' : 'status-pending';
            html += `
                <div class="progress-item">
                    <span>${type}</span>
                    <span class="status-badge ${progressClass}">${status}</span>
                </div>
            `;
        }
        html += `</div>`;
    }
    
    // 输出文件
    if (task.output_files && Object.keys(task.output_files).length > 0) {
        html += `<h4 style="margin-top:20px;margin-bottom:10px;">输出文件</h4><div class="progress-list">`;
        for (const [type, path] of Object.entries(task.output_files)) {
            html += `
                <div class="progress-item">
                    <span>${type}</span>
                    <span style="word-break:break-all;">${path}</span>
                </div>
            `;
        }
        html += `</div>`;
    }
    
    // 错误信息
    if (task.error_message) {
        html += `
            <div class="error-box">
                <h4>❌ 错误信息</h4>
                <p>${task.error_message}</p>
            </div>
        `;
    }
    
    // 错误详情
    if (task.error_details && task.error_details.length > 0) {
        html += `<h4 style="margin-top:20px;margin-bottom:10px;">错误详情</h4>`;
        task.error_details.forEach((detail, index) => {
            html += `
                <div class="error-box" style="margin-top:10px;">
                    <h4>错误 ${index + 1}: ${detail.transcode_type} - ${detail.stage}</h4>
                    <p><strong>错误:</strong> ${detail.error}</p>
                    ${detail.command ? `<p><strong>命令:</strong> <code style="word-break:break-all;">${detail.command}</code></p>` : ''}
                    ${detail.output ? `<pre style="background:#f3f4f6;padding:10px;border-radius:4px;overflow-x:auto;font-size:12px;max-height:200px;">${escapeHtml(detail.output)}</pre>` : ''}
                </div>
            `;
        });
    }
    
    // 操作按钮
    const canRerun = task.status !== 'processing';
    const canCancel = task.status === 'pending';
    const canAbort = task.status === 'processing';
    
    html += `
        <div style="margin-top:24px;display:flex;gap:12px;">
            ${canRerun ? `<button class="btn btn-primary" onclick="retryTask('${task.task_id}');closeModal();">🔄 重新运行</button>` : ''}
            ${canCancel ? `<button class="btn btn-danger" onclick="cancelTask('${task.task_id}');closeModal();">❌ 取消任务</button>` : ''}
            ${canAbort ? `<button class="btn btn-danger" onclick="abortTask('${task.task_id}');closeModal();">⛔ 中止任务</button>` : ''}
            <button class="btn btn-secondary" onclick="closeModal()">关闭</button>
        </div>
    `;
    
    return html;
}

// 关闭模态框
function closeModal() {
    document.getElementById('taskDetailModal').classList.remove('active');
}

// 重新运行任务
async function retryTask(taskId) {
    if (!confirm('确定要重新运行此任务吗？这将重置任务状态并重新加入队列。')) return;
    
    try {
        const res = await fetch(`${API_BASE}/tasks/${taskId}/retry`, { method: 'POST' });
        const data = await res.json();
        
        if (res.ok) {
            showToast('任务已重新加入队列', 'success');
            loadTasks();
            loadDashboard();
        } else {
            showToast(data.error || '重新运行失败', 'error');
        }
    } catch (e) {
        showToast('重新运行任务失败', 'error');
    }
}

// 取消任务（等待中的任务）
async function cancelTask(taskId) {
    if (!confirm('确定要取消此任务吗？')) return;
    
    try {
        const res = await fetch(`${API_BASE}/tasks/${taskId}`, { method: 'DELETE' });
        const data = await res.json();
        
        if (res.ok) {
            showToast('任务已取消', 'success');
            loadTasks();
            loadDashboard();
        } else {
            showToast(data.error || '取消失败', 'error');
        }
    } catch (e) {
        showToast('取消任务失败', 'error');
    }
}

// 中止任务（正在运行的任务）
async function abortTask(taskId) {
    if (!confirm('⚠️ 确定要中止此正在运行的任务吗？任务将被标记为失败状态。')) return;
    
    try {
        const res = await fetch(`${API_BASE}/tasks/${taskId}/abort`, { method: 'POST' });
        const data = await res.json();
        
        if (res.ok) {
            showToast('任务已中止', 'success');
            loadTasks();
            loadDashboard();
        } else {
            showToast(data.error || '中止失败', 'error');
        }
    } catch (e) {
        showToast('中止任务失败', 'error');
    }
}

// 刷新队列状态
async function refreshQueueStatus() {
    await loadQueueStats();
    showToast('队列状态已刷新', 'success');
}

// 清空队列
async function purgeQueue() {
    if (!confirm('⚠️ 确定要清空队列吗？此操作不可恢复！')) return;
    
    try {
        const res = await fetch(`${API_BASE}/queue/purge`, { method: 'DELETE' });
        const data = await res.json();
        
        if (res.ok) {
            showToast('队列已清空', 'success');
            loadQueueStats();
        } else {
            showToast(data.error || '清空队列失败', 'error');
        }
    } catch (e) {
        showToast('清空队列失败', 'error');
    }
}

// 提交任务
async function submitTask(event) {
    event.preventDefault();
    
    const inputBucket = document.getElementById('inputBucket').value.trim();
    const inputKey = document.getElementById('inputKey').value.trim();
    const checkboxes = document.querySelectorAll('input[name="transcodeType"]:checked');
    
    if (checkboxes.length === 0) {
        showToast('请至少选择一种转码类型', 'error');
        return;
    }
    
    const transcodeTypes = Array.from(checkboxes).map(cb => cb.value);
    
    try {
        const res = await fetch(`${API_BASE}/queue/add`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                input_bucket: inputBucket,
                input_key: inputKey,
                transcode_types: transcodeTypes
            })
        });
        
        const data = await res.json();
        
        if (res.ok) {
            showToast(`任务创建成功: ${data.task_id}`, 'success');
            document.getElementById('addTaskForm').reset();
            loadDashboard();
        } else {
            showToast(data.error || '创建任务失败', 'error');
        }
    } catch (e) {
        showToast('创建任务失败', 'error');
    }
}

// 工具函数
function getStatusText(status) {
    const map = {
        'pending': '等待中',
        'processing': '处理中',
        'completed': '已完成',
        'failed': '失败',
        'retrying': '重试中',
        'cancelled': '已取消'
    };
    return map[status] || status;
}

function formatDate(dateStr) {
    if (!dateStr) return '-';
    const date = new Date(dateStr);
    return date.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
    });
}

function truncate(str, len) {
    if (!str) return '-';
    return str.length > len ? str.substring(0, len) + '...' : str;
}

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
    
    setTimeout(() => {
        toast.remove();
    }, 3000);
}

// 点击模态框外部关闭
document.getElementById('taskDetailModal').addEventListener('click', (e) => {
    if (e.target.id === 'taskDetailModal') {
        closeModal();
    }
});

// ==================== 表格列宽拖拽调整功能 ====================

// 初始化表格列宽调整
function initTableResize() {
    // 使用 MutationObserver 监听表格变化，自动添加拖拽手柄
    const observer = new MutationObserver(() => {
        document.querySelectorAll('.data-table').forEach(table => {
            if (!table.dataset.resizeInit) {
                setupTableResize(table);
                table.dataset.resizeInit = 'true';
            }
        });
    });
    
    observer.observe(document.body, { childList: true, subtree: true });
    
    // 初始化已存在的表格
    document.querySelectorAll('.data-table').forEach(table => {
        setupTableResize(table);
        table.dataset.resizeInit = 'true';
    });
}

// 为单个表格设置列宽调整
function setupTableResize(table) {
    const headerCells = table.querySelectorAll('th');
    
    headerCells.forEach((th, index) => {
        // 跳过最后一列（操作列）
        if (index === headerCells.length - 1) return;
        
        // 检查是否已添加手柄
        if (th.querySelector('.resize-handle')) return;
        
        // 创建拖拽手柄
        const handle = document.createElement('div');
        handle.className = 'resize-handle';
        th.appendChild(handle);
        
        // 拖拽事件
        let startX, startWidth, columnIndex;
        
        handle.addEventListener('mousedown', (e) => {
            e.preventDefault();
            e.stopPropagation();
            startX = e.pageX;
            startWidth = th.offsetWidth;
            columnIndex = index;
            
            handle.classList.add('resizing');
            table.classList.add('resizing');
            
            document.addEventListener('mousemove', onMouseMove);
            document.addEventListener('mouseup', onMouseUp);
        });
        
        function onMouseMove(e) {
            const diff = e.pageX - startX;
            const newWidth = Math.max(80, startWidth + diff);
            
            // 设置表头宽度并添加 resized 类
            th.style.width = newWidth + 'px';
            th.classList.add('resized');
            
            // 同步调整对应列的所有单元格
            const rows = table.querySelectorAll('tbody tr');
            rows.forEach(row => {
                const cells = row.querySelectorAll('td');
                if (cells[columnIndex]) {
                    cells[columnIndex].style.width = newWidth + 'px';
                    cells[columnIndex].classList.add('resized');
                }
            });
        }
        
        function onMouseUp() {
            handle.classList.remove('resizing');
            table.classList.remove('resizing');
            document.removeEventListener('mousemove', onMouseMove);
            document.removeEventListener('mouseup', onMouseUp);
        }
    });
}

// 重新初始化表格（数据更新后调用）
function reinitTableResize() {
    document.querySelectorAll('.data-table').forEach(table => {
        table.dataset.resizeInit = '';
        setupTableResize(table);
        table.dataset.resizeInit = 'true';
    });
}
