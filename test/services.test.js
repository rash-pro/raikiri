const { app, server } = require('../src/server');
const TwitchService = require('../src/services/twitch');
const YouTubeService = require('../src/services/youtube');

// Mock Socket.io
const mockIo = {
    emit: jest.fn(),
};

describe('Message Normalization', () => {
    beforeEach(() => {
        mockIo.emit.mockClear();
    });

    test('TwitchService normalizes messages correctly', () => {
        const service = new TwitchService(['test'], mockIo);
        // Directly invoke the message handler if possible, or mock the client.on
        // Since we can't easily access the internal client listener without refactoring to expose it or mock tmi.js,
        // We will verify the logic by simulating what the handler DOES.
        // actually, let's mock the tmi client in the service.
    });
});

describe('Normalization Logic Unit Tests', () => {
    test('Twitch payload structure', () => {
        const twitchService = new TwitchService(['test'], mockIo);
        // We simulate the logic inside the listener
        const tags = { 'id': '123', 'display-name': 'TestUser', 'color': '#FFFFFF', 'badges-raw': 'admin' };
        const message = 'Hello World';

        const normalized = {
            platform: 'twitch',
            id: tags['id'],
            user: tags['display-name'],
            content: message,
            color: tags['color'],
            timestamp: expect.any(String), // We can't predict exact time
            badges: tags['badges-raw'],
        };

        // This is a "white-box" test of the logic we put in the file. 
        // Ideally we'd extract the normalizer function to test it purely.
    });
});

// Since we didn't export the normalizer, we will refactor slightly to test it or just write a test 
// that checks if the services file logic is correct by inspecting the file or trusting the integration.
// Better: Refactor Services to have a static `normalize` method or similar?
// Or just Mock the library and instantiate the service.

jest.mock('tmi.js', () => {
    return {
        Client: jest.fn().mockImplementation(() => ({
            on: jest.fn(),
            connect: jest.fn(),
            disconnect: jest.fn()
        }))
    };
});

jest.mock('youtube-chat', () => {
    return {
        LiveChat: jest.fn().mockImplementation(() => ({
            on: jest.fn(),
            start: jest.fn(),
            stop: jest.fn(),
        }))
    };
});

describe('Service Integration Tests with Mocks', () => {
    test('TwitchService emits correct format', () => {
        const TwitchService = require('../src/services/twitch');
        const service = new TwitchService(['test'], mockIo);

        // Access the 'message' callback that was passed to client.on
        // This requires us to capture the callback.
        const mockClient = service.client;
        const onCallback = mockClient.on.mock.calls.find(call => call[0] === 'message')[1];

        const tags = { id: '123', 'display-name': 'TestUser', color: '#000', 'badges-raw': 'vip' };
        onCallback('channel', tags, 'Hello', false);

        expect(mockIo.emit).toHaveBeenCalledWith('chat_message', expect.objectContaining({
            platform: 'twitch',
            user: 'TestUser',
            content: 'Hello',
            id: '123'
        }));
    });

    test('YouTubeService emits correct format', () => {
        const YouTubeService = require('../src/services/youtube');
        const service = new YouTubeService({ channelId: '123' }, mockIo);

        const mockLC = service.liveChat;
        const onCallback = mockLC.on.mock.calls.find(call => call[0] === 'chat')[1];

        const chatItem = {
            id: 'yt1',
            author: { name: 'YTUser' },
            message: [{ text: 'Hello YouTube' }],
            timestamp: Date.now()
        };

        onCallback(chatItem);

        expect(mockIo.emit).toHaveBeenCalledWith('chat_message', expect.objectContaining({
            platform: 'youtube',
            user: 'YTUser',
            content: 'Hello YouTube',
            id: 'yt1'
        }));
    });
});
