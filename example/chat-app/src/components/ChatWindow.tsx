import React, { useEffect, useState, useRef } from 'react';
import { getMessagesCollection, getToolCallsCollection, startChatSync, startToolCallSync, Message, ToolCall } from '../db';
import { MessageInput } from './MessageInput';

interface ChatWindowProps {
  chatId: string;
}

type ChatItem =
  | (Message & { type: 'message' })
  | (ToolCall & { type: 'toolcall' });

const ToolCallItem: React.FC<{ item: ToolCall }> = ({ item }) => {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <div className={`tool-call-box ${item.status}`}>
      <div className="tool-header" onClick={() => setIsOpen(!isOpen)} style={{ cursor: 'pointer' }}>
        <span className="tool-name">
          {isOpen ? '▼' : '▶'} 🔧 {item.toolName}
        </span>
        <span className={`tool-status ${item.status}`}>{item.status}</span>
      </div>
      {isOpen && (
        <div className="tool-body">
          <div className="tool-args">
            <pre>{JSON.stringify(item.args, null, 2)}</pre>
          </div>
          {item.result && (
            <div className="tool-result">
              <strong>Result:</strong>
              <pre>{item.result}</pre>
            </div>
          )}
          {item.error && (
            <div className="tool-error">
              <strong>Error:</strong> {item.error}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export const ChatWindow: React.FC<ChatWindowProps> = ({ chatId }) => {
  const [items, setItems] = useState<ChatItem[]>([]);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let isMounted = true;
    let msgSub: any;
    let toolSub: any;
    let msgSyncCancel: (() => void) | undefined;
    let toolSyncCancel: (() => void) | undefined;

    const init = async () => {
      const msgCollection = await getMessagesCollection(chatId);
      const toolCollection = await getToolCallsCollection(chatId);

      if (!isMounted) return;

      // Combine subscriptions
      let currentMessages: Message[] = [];
      let currentTools: ToolCall[] = [];

      const updateItems = () => {
        if (!isMounted) return;
        const combined: ChatItem[] = [
          ...currentMessages.map(m => ({ ...m, type: 'message' } as const)),
          ...currentTools.map(t => ({ ...t, type: 'toolcall' } as const))
        ];
        combined.sort((a, b) => a.createdAt - b.createdAt);
        setItems(combined);
      };

      msgSub = msgCollection.find().sort({ createdAt: 'asc' }).$.subscribe(msgs => {
        currentMessages = msgs.map(m => m.toJSON());
        updateItems();
      });

      toolSub = toolCollection.find().sort({ createdAt: 'asc' }).$.subscribe(tools => {
        currentTools = tools.map(t => t.toJSON());
        updateItems();
      });

      // Start Syncs
      const { cancel: cancelMsg } = await startChatSync(chatId);
      msgSyncCancel = cancelMsg;

      const { cancel: cancelTool } = await startToolCallSync(chatId);
      toolSyncCancel = cancelTool;
    };

    init();

    return () => {
      isMounted = false;
      if (msgSub) msgSub.unsubscribe();
      if (toolSub) toolSub.unsubscribe();
      if (msgSyncCancel) msgSyncCancel();
      if (toolSyncCancel) toolSyncCancel();
    };
  }, [chatId]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [items]);

  return (
    <div className="chat-window">
      <div className="messages">
        {items.map(item => {
          if (item.type === 'message') {
            return (
              <div key={item.id} className={`message ${item.role}`}>
                <div className="content">{item.content}</div>
              </div>
            );
          } else {
            return <ToolCallItem key={item.id} item={item} />;
          }
        })}
        <div ref={messagesEndRef} />
      </div>
      <MessageInput chatId={chatId} />
    </div>
  );
};
