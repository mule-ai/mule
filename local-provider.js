// local-provider.js

function showEditIssueModal(issueNumber, title, body) {
    document.getElementById('editTitle').value = title;
    document.getElementById('editBody').value = body;
}

async function handleEditIssue(event) {
    event.preventDefault();
    
    const form = document.getElementById('editIssueForm');
    const issueNumber = parseInt(document.getElementById('issueNumber').value);
    const newTitle = document.getElementById('editTitle').value;
    const newBody = document.getElementById('editBody').value;
    const path = document.getElementById('repoPath').value;

    try {
        const response = await fetch('/api/local/issues', {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                path,
                issueNumber,
                title: newTitle,
                body: newBody
            })
        });

        if (!response.ok) throw new Error(await response.text());

        closeModal('editIssueModal');
        updateIssueDisplay(issueNumber, newTitle, newBody);
    } catch (err) {
        alert(`Error saving changes: ${err.message}`);
    }
}

function updateIssueDisplay(number, title, body) {
    const issueElement = document.getElementById(`issue-${number}`);
    if (issueElement) {
        issueElement.querySelector('.issue-title').innerText = `#${number} ${title}`;
        issueElement.querySelector('.issue-body').innerText = body;
    }
}
