class WebSocketService {
  constructor() {
    this.ws = null;
    this.listeners = {};
  }

  connect() {
    // Close any existing connection
    if (this.ws) {
      this.ws.close();
    }

    // Determine WebSocket URL based on current protocol and host
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    console.log(`Attempting to connect to WebSocket at ${wsUrl}`);

    try {
      this.ws = new WebSocket(wsUrl);
      this.setupEventHandlers();
    } catch (error) {
      console.error('Failed to create WebSocket connection:', error);
      this.publish('connection_status', { connected: false });
    }
  }

  setupEventHandlers() {
    this.ws.onopen = () => {
      console.log('WebSocket connected');
      this.publish('connection_status', { connected: true });
    };

    this.ws.onclose = (event) => {
      console.log('WebSocket closed', event);
      this.publish('connection_status', { connected: false });
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
      // Publish connection status as false on error
      this.publish('connection_status', { connected: false });
      this.publish('connection_error', { error });
    };

    this.ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        this.handleMessage(message);
      } catch (error) {
        console.error('Error parsing WebSocket message:', error);
      }
    };
  }

  handleMessage(message) {
    // Publish the message to all subscribers of its type
    this.publish(message.type, message.data);

    // Also publish to general message subscribers
    this.publish('message', message);
  }

  subscribe(eventType, callback) {
    if (!this.listeners[eventType]) {
      this.listeners[eventType] = [];
    }

    this.listeners[eventType].push(callback);
  }

  unsubscribe(eventType, callback) {
    if (!this.listeners[eventType]) return;

    const index = this.listeners[eventType].indexOf(callback);
    if (index > -1) {
      this.listeners[eventType].splice(index, 1);
    }
  }

  publish(eventType, data) {
    if (!this.listeners[eventType]) return;

    // Call all listeners for this event type
    this.listeners[eventType].forEach(callback => {
      try {
        callback(data);
      } catch (error) {
        console.error('Error in WebSocket listener:', error);
      }
    });
  }

  disconnect() {
    if (this.ws) {
      this.ws.close();
    }
  }

  isConnected() {
    return this.ws && this.ws.readyState === WebSocket.OPEN;
  }
}

// Export singleton instance
const webSocketService = new WebSocketService();
export default webSocketService;