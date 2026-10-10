<!-- Template for agent settings -->
<div>
  <h2>Agent Settings</h2>
  <form>
    <label>Provider:</label>
    <select>
      <option value="openai">OpenAI</option>
      <option value="anthropic">Anthropic</option>
      <option value="google">Google</option>
      <option value="local">Local</option>
    </select>
    <br>
    <label>Model:</label>
    <select>
      <option value="gpt-4">GPT-4</option>
      <option value="gpt-3.5-turbo">GPT-3.5-turbo</option>
      <option value="claud-3">Claude-3</option>
      <option value="gemini-pro">Gemini Pro</option>
      <option value="ollama">Ollama</option>
    </select>
    <br>
    <label>Temperature:</label>
    <input type="number" value="0.1">
    <br>
    <label>Max Tokens:</label>
    <input type="number" value="2000">
    <br>
    <label>Top P:</label>
    <input type="number" value="0.9">
    <br>
    <label>Frequency Penalty:</label>
    <input type="number" value="0.0">
    <br>
    <label>Presence Penalty:</label>
    <input type="number" value="0.0">
    <br>
    <label>Stop Sequences:</label>
    <input type="text" value="END_REVIEW,---">
    <br>
    <label>System Prompt:</label>
    <textarea> You are an expert code reviewer... </textarea>
    <br>
    <button type="submit">Save Settings</button>
  </form>
</div>