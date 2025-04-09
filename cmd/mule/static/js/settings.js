function updateWorkflowDiagram(workflowIndex) {
    const workflowSteps = document.querySelectorAll(`.workflow-steps[data-workflow-index="${workflowIndex}"] .workflow-step`);
    const diagramContainer = document.querySelector(`.workflow-diagram-steps[data-workflow-diagram="${workflowIndex}"]`);
    
    if (!diagramContainer) return;
    
    // Clear existing diagram
    diagramContainer.innerHTML = '';
    
    if (workflowSteps.length === 0) {
        diagramContainer.innerHTML = '<div class="empty-diagram">No steps defined yet</div>';
        return;
    }
    
    // Create diagram elements
    workflowSteps.forEach((step, index) => {
        const stepIndex = step.getAttribute('data-step-index');
        const agentNameElement = step.querySelector('.step-agent-name');
        const agentName = agentNameElement ? agentNameElement.textContent : 'Unnamed Agent';
        const isFirst = step.querySelector('input[name$=".isFirst"]').checked;
        const outputField = step.querySelector('select[name$=".outputField"]').value;
        const inputMapping = !isFirst ? step.querySelector('select[name$=".inputMapping"]').value : 'N/A';
        
        // Create step node
        const stepNode = document.createElement('div');
        stepNode.className = 'diagram-step';
        stepNode.innerHTML = `
            <div class="diagram-step-number">${parseInt(stepIndex) + 1}</div>
            <div class="diagram-step-content">
                <div class="diagram-agent-name">${agentName}</div>
                <div class="diagram-step-details">
                    ${isFirst ? '<span class="diagram-first-step">First Step</span>' : 
                      `<span class="diagram-input">Input: ${inputMapping.replace(/([A-Z])/g, ' $1').replace(/^./, str => str.toUpperCase())}</span>`}
                    <span class="diagram-output">Output: ${outputField.replace(/([A-Z])/g, ' $1').replace(/^./, str => str.toUpperCase())}</span>
                </div>
            </div>
        `;
        
        // Create connector if not the last step
        if (index < workflowSteps.length - 1) {
            const connector = document.createElement('div');
            connector.className = 'diagram-connector';
            connector.innerHTML = '<svg viewBox="0 0 24 24"><path d="M16.01 11H4v2h12.01v3L20 12l-3.99-4v3z"></path></svg>';
            diagramContainer.appendChild(stepNode);
            diagramContainer.appendChild(connector);
        } else {
            diagramContainer.appendChild(stepNode);
        }
    });
}


    e.preventDefault();
    const issueNumber = document.getElementById('issue-number-to-edit').value;
    try {
        // Fetch issue content from backend
        const response = await fetch(`/api/issues/local/${issueNumber}`, {
            method: 'GET'
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
function showLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'block');

// Handle local issue editing form submission
document.getElementById('local-edit-form').addEventListener('submit', async (e) => {

        // Fetch issue content from backend
        const response = await fetch(`/api/issues/local/${issueNumber}`, {
            method: 'GET'
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
        }
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
}

    e.preventDefault();
    const issueNumber = document.getElementById('issue-number-to-edit').value;
    try {
        // Fetch issue content from backend
            document.getElementById('issue-content').value = data.content;
        }
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
}
        const response = await fetch(`/api/issues/local/${issueNumber}`, {
            method: 'GET'
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');

    e.preventDefault();
    const issueNumber = document.getElementById('issue-number-to-edit').value;
    try {
        // Fetch issue content from backend
        const response = await fetch(`/api/issues/local/${issueNumber}`, {
            method: 'GET'
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
function showLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'block');

// Handle local issue editing form submission
document.getElementById('local-edit-form').addEventListener('submit', async (e) => {

    e.preventDefault();
    const issueNumber = document.getElementById('issue-number-to-edit').value;
    try {
        // Fetch issue content from backend
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
        }
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
function showLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'block');

// Handle local issue editing form submission
document.getElementById('local-edit-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const issueNumber = document.getElementById('issue-number-to-edit').value;
    try {
        // Fetch issue content from backend
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
        }

    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
}

        const response = await fetch(`/api/issues/local/${issueNumber}`, {
            method: 'GET'
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
        }
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
}

    e.preventDefault();
    const issueNumber = document.getElementById('issue-number-to-edit').value;
    try {
        // Fetch issue content from backend
        const response = await fetch(`/api/issues/local/${issueNumber}`, {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
        }
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
function showLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'block');

// Handle local issue editing form submission
document.getElementById('local-edit-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const issueNumber = document.getElementById('issue-number-to-edit').value;
    try {
        // Fetch issue content from backend
        const response = await fetch(`/api/issues/local/${issueNumber}`, {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
        }
    } catch (error) {

}

            method: 'GET'
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
        }
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
}
    e.preventDefault();
    const issueNumber = document.getElementById('issue-number-to-edit').value;
    try {
        // Fetch issue content from backend
        const response = await fetch(`/api/issues/local/${issueNumber}`, {
            method: 'GET'
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
function initWorkflowDiagrams() {
    const workflows = document.querySelectorAll('.workflow');
    workflows.forEach((workflow, index) => {
        updateWorkflowDiagram(index);
    });
}

// Update diagram when steps change
function addWorkflowStep(workflowIndex) {
    const workflowSteps = document.querySelector(`.workflow-steps[data-workflow-index="${workflowIndex}"]`);
    const stepCount = workflowSteps.querySelectorAll('.workflow-step').length;
    
    const newStep = document.createElement('div');
    newStep.className = 'workflow-step';
    newStep.setAttribute('data-step-index', stepCount);
    
    newStep.innerHTML = `
        <div class="workflow-step-header">
            <span class="step-number">${stepCount + 1}</span>
            <div class="step-agent-name">Select an agent</div>
            <button type="button" class="button secondary remove-step" onclick="removeWorkflowStep(this)">×</button>
        </div>
        <div class="workflow-step-content">
            <input type="hidden" name="workflows[${workflowIndex}].steps[${stepCount}].id" value="">
            <div class="form-group">
                <label class="label">Agent</label>
                <select name="workflows[${workflowIndex}].steps[${stepCount}].agentName" class="input agent-select" onchange="updateStepAgentName(this)">
                    <option value="">Select an agent</option>
                    ${Array.from(document.querySelectorAll(`select[name="workflows[${workflowIndex}].steps[0].agentName"] option`))
                        .map(opt => opt.outerHTML)
                        .join('')}
                </select>
function showLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'block');
}

// Handle local issue editing form submission
document.getElementById('local-edit-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const issueNumber = document.getElementById('issue-number-to-edit').value;
    try {
        // Fetch issue content from backend
        const response = await fetch(`/api/issues/local/${issueNumber}`, {
            method: 'GET'
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
        }
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
}
            </div>
            <div class="form-group">
                <label class="checkbox-item">
                    <input type="checkbox" name="workflows[${workflowIndex}].steps[${stepCount}].isFirst" onchange="handleFirstStepChange(this)">
                    First step in workflow
                </label>
            </div>
            <div class="form-group">
                <label class="label">Input Mapping</label>
                <select name="workflows[${workflowIndex}].steps[${stepCount}].inputMapping" class="input input-mapping-select">
                    <option value="">Select input mapping</option>
                    <option value="useAsPrompt">Use as prompt</option>
                    <option value="appendToPrompt">Append to prompt</option>
                    <option value="useAsContext">Use as context</option>
                    <option value="useAsInstructions">Use as instructions</option>
                    <option value="useAsCodeInput">Use as code input</option>
                    <option value="useAsReviewTarget">Use as review target</option>
                </select>
            </div>
            <div class="form-group">
                <label class="label">Output Field</label>
                <select name="workflows[${workflowIndex}].steps[${stepCount}].outputField" class="input output-field-select">
                    <option value="">Select output field</option>
function showLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'block');
}

// Handle local issue editing form submission
document.getElementById('local-edit-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const issueNumber = document.getElementById('issue-number-to-edit').value;
    try {
        // Fetch issue content from backend
        const response = await fetch(`/api/issues/local/${issueNumber}`, {
            method: 'GET'
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('issue-content').value = data.content;
        }
    } catch (error) {
        console.error('Error fetching issue:', error);
    }

function hideLocalEditUI() {
    document.querySelectorAll('.edit-local-issue').forEach(el => el.style.display = 'none');
}
                    <option value="generatedText">Generated Text</option>
                    <option value="extractedCode">Extracted Code</option>
                    <option value="summary">Summary</option>
                    <option value="actionItems">Action Items</option>
                    <option value="suggestedChanges">Suggested Changes</option>
                    <option value="reviewComments">Review Comments</option>
                    <option value="testCases">Test Cases</option>
                    <option value="documentationText">Documentation Text</option>
                </select>
            </div>
        </div>
    `;
    
    workflowSteps.appendChild(newStep);
    updateWorkflowDiagram(workflowIndex);
}

function removeWorkflowStep(button) {
    const step = button.closest('.workflow-step');
    const workflowSteps = step.closest('.workflow-steps');
    const workflowIndex = workflowSteps.getAttribute('data-workflow-index');
    const stepIndex = parseInt(step.getAttribute('data-step-index'));
    
    // Remove the step
    step.remove();
    
    // Update indices for remaining steps
    const remainingSteps = workflowSteps.querySelectorAll('.workflow-step');
    remainingSteps.forEach((remainingStep, newIndex) => {
        // Update step number display
        remainingStep.querySelector('.step-number').textContent = newIndex + 1;
        
        // Update data attribute
        remainingStep.setAttribute('data-step-index', newIndex);
        
        // Update form field names
        const formFields = remainingStep.querySelectorAll('[name^="workflows["]');
        formFields.forEach(field => {
            const name = field.getAttribute('name');
            const updatedName = name.replace(/steps\[\d+\]/, `steps[${newIndex}]`);
            field.setAttribute('name', updatedName);
        });
    });
    
    updateWorkflowDiagram(workflowIndex);
}

function handleFirstStepChange(checkbox) {
    const step = checkbox.closest('.workflow-step');
    const workflowSteps = step.closest('.workflow-steps');
    const workflowIndex = workflowSteps.getAttribute('data-workflow-index');
    
    if (checkbox.checked) {
        // Uncheck all other "first step" checkboxes in this workflow
        const otherCheckboxes = workflowSteps.querySelectorAll('input[name$=".isFirst"]');
        otherCheckboxes.forEach(otherCheckbox => {
            if (otherCheckbox !== checkbox) {
                otherCheckbox.checked = false;
                
                // Show input mapping for steps that are not first
                const inputMappingGroup = otherCheckbox.closest('.workflow-step').querySelector('.form-group:nth-child(3)');
                if (inputMappingGroup) {
                    inputMappingGroup.style.display = 'block';
                }
            }
        });
        
        // Hide input mapping for first step
        const inputMappingGroup = step.querySelector('.form-group:nth-child(3)');
        if (inputMappingGroup) {
            inputMappingGroup.style.display = 'none';
        }
    } else {
        // Show input mapping when unchecked
        const inputMappingGroup = step.querySelector('.form-group:nth-child(3)');
        if (inputMappingGroup) {
            inputMappingGroup.style.display = 'block';
        }
    }
    
    updateWorkflowDiagram(workflowIndex);
}

function updateStepAgentName(select) {
    const step = select.closest('.workflow-step');
    const agentName = select.options[select.selectedIndex].text;
    step.querySelector('.step-agent-name').textContent = agentName;
    
    const workflowIndex = select.closest('.workflow-steps').getAttribute('data-workflow-index');
    updateWorkflowDiagram(workflowIndex);
}

function handleUDiffSettingsChange(checkbox) {
    // This function is a placeholder for future enhancements
    // Currently just handles the checkbox state
    console.log("UDiff settings changed:", checkbox.checked);
}

function removeWorkflow(button) {
    const workflow = button.closest('.workflow');
    const workflowContainer = workflow.closest('.workflow-container');
    const workflowIndex = Array.from(workflowContainer.querySelectorAll('.workflow')).indexOf(workflow);
    
    // Remove the workflow
    workflow.remove();
    
    // Update indices for remaining workflows
    const remainingWorkflows = workflowContainer.querySelectorAll('.workflow');
    remainingWorkflows.forEach((remainingWorkflow, newIndex) => {
        // Update data attributes
        remainingWorkflow.querySelectorAll('[data-workflow-index]').forEach(el => {
            el.setAttribute('data-workflow-index', newIndex);
        });
        
        remainingWorkflow.querySelectorAll('[data-workflow-diagram]').forEach(el => {
            el.setAttribute('data-workflow-diagram', newIndex);
        });
        
        // Update form field names
        const formFields = remainingWorkflow.querySelectorAll('[name^="workflows["]');
        formFields.forEach(field => {
            const name = field.getAttribute('name');
            const updatedName = name.replace(/workflows\[\d+\]/, `workflows[${newIndex}]`);
            field.setAttribute('name', updatedName);
        });
        
        // Update workflow title
        const workflowTitle = remainingWorkflow.querySelector('.workflow-title');
        if (workflowTitle) {
            workflowTitle.textContent = `Workflow ${newIndex + 1}`;
        }
    });
}
