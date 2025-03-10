// local-provider.js

function showEditIssueModal(issueNumber, title, body) {
    document.getElementById('editTitle').value = title;
    document.getElementById('editBody').value = body;
}

function handleEditIssue(event) {
    event.preventDefault();

    const issueNumber = document.getElementById('issueNumber').value;
    const newTitle = document.getElementById('editTitle').value;
    const newBody = document.getElementById('editBody').value;

    fetch('/api/local/issues', {
        method: 'PUT',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({
            path: '/data/jbutler/tmpgit/mule-ai/mule/repositories/local-repo',
            issueNumber: parseInt(issueNumber),
            title: newTitle,
            body: newBody
        })
    }).then(response => {
        if (response.ok) {
            updateIssueDisplay(issueNumber, newTitle, newBody);
            document.getElementById('editModal').style.display = 'none';
            alert('Issue updated successfully');
        } else {
            response.text().then(text => alert(`Error: ${text}`));
        }
    }).catch(error => alert(`Error: ${error}`));
}

function updateIssueDisplay(issueNumber, newTitle, newBody) {
    const issueElement = document.getElementById(`issue-${issueNumber}`);
    if (issueElement) {
        issueElement.querySelector('.issue-title').innerText = `#${issueNumber} ${newTitle}`;
        issueElement.querySelector('.issue-body').innerText = newBody;
    }
}
