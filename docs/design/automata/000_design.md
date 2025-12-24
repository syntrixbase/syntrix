# Automabase

在syntrix里加入一个基于**自动机原理**来设计的单向数据流的触发式实时数据库.

## 名词

- `Stream`: 一个事件流
- `Automata`: 自动机,一个状态机
- `RootDocument`: 存放整个自动机的状态
- `Channel`: 从`Stream`事件流分出来子流,过滤出必要的信息并变换成需要的形态
- `Reducer`: 一个无状态的函数, 从`Channel`迭代出一个`State`
- `State`: 基于`Channel`迭代的结果, 有初始状态
- `Processor`: 状态处理机, 可以关注多个`Channel`或者`State`, 在其发生变化时被调用

## Automata的结构

- 指定一个`Stream`来绑定到`Automata`作为输入, 比如: `apps/chatbot/users/{user-id}/chats/{chat-id}`
- 指定一个`RootDocument`来绑定到`Automata`来存放状态, 比如: `apps/chatbot/users/{user-id}/chats/{chat-id}/messages`
- 用户可以设计一个`Automata`, 包括:
  - 一个默认`Channel`, 包含所有的`Stream`
  - 一些`Named Channel`, 每个都是`Stream + filter`得到的结果
  - 一些`Named State`, 通过`Reducer`来得到一个确定的状态(不是一个array或者list), 每个`State`只能关注一个`Channel`
  - 一些`Named Processor`, `Processor`可以关注多个`Channel`和`State`的变化

## 工作机制

`Automata`只有一个`Stream`作为输入驱动整个状态机运行, 当有`Stream`来的时候:

- 触发`Channel`更新
- 继而触发`Reducer`执行, 迭代出新的`State`, 并缓存结构
- `Channel`更新或者`State`变化会触发关注它们的`Processor`执行

## 数据模型

- Stream: 有序切不可变的事件流

- `RootDocument`: `.../{automata}`, 自动机的根文档, 也存储自动机元数据, 整个自动机都是以它为parent

  ```json
  {
    "automaManaged": true,         // 表示这个是Automata托管数据
    "input": "<input collection>", // e.g.: apps/chatbot/users/{user-id}/chats/{chat-id}/messages
    "..."
  }
  ```

- `.../{automata}/channels/{channel name}/results`: 存放Named Channel的结果集

  - 这是一个Collection, 里面可以放很多document, 按固定字段(比如:timestamp)排序可以得到一个有序的列表
  - `{channel name}` 记录Channel执行的状态:

  ```json
  {
    "automaManaged": true,
    "progress": "<progress token>", // 记录处理stream的进度
    "...": "...", // 更多其他的状态
  }
  ```

- `Named State`结果放到: `.../{automata}/states/{state name}`

  `{state name}` 记录当前`State`

  ```json
  {
    "automaManaged": true,
    "progress": "<progress token>", // 记录处理view的进度
    "state": {...}, // current state 缓存
  }
  ```

- `Named Processor`: `.../{automata}/processors/{processor name}`

  `{processor name}` 记录`Processor`执行状态:

  ```json
  {
    "automaManaged": true,
    "channels": {
      "name": "<progress token>",
      // ...
    },
    "states": {
      "name": "<progress token>",
      // ...
    }
  }
  ```

## 工作方式

- `Channel`: 一组`Stream`上的过滤器, 生成一个数据库层视图, 它由`Automabase`处理, 不需要回调
- `Reducer`: Webhook (也可以是托管的JSONata), 业务定义的一个无状态函数, 从`Channel`生成一个`State`, 需要严格控制处理时间:
  - 输入:{Current State} + {Channel Stream Delta}

  ```json
  {
    "currentState": {...},
    "changes": [{...}, ...]
  }
  ```

  - 输出:
    - Success + {New State} Or
    - Failed, 一会重试

    ```json
    {
      "status": Success|Failed,
      "newState": {...} // 当status=Success时存在
    }
    ```

- `Processor`: `Webhook`, 业务定义的复杂运算,定义为有自己状态,当收到`webhook`后需要记录状态并**快速ack**:
  - 输入:

    ```json
    {
      "channels": {
        "{name}": [{<change>}, ...],
        // ...
      },
      "states": {
        "name": {<New State>},
        // ...
      }
    }
    ```

  - 输出: Success or Fail

    返回Success表示Processor以成功收到了change, 会在后台慢慢处理, 处理后对状态机新输入会append到Stream里。

## Restful接口

### 管理Automata

- Create: `POST /automata/v1/create`
- Get: `GET /automata/v1/<name>`
- Query: `GET /automata/v1/?<filters>`
- Update: `PUT /automata/v1/<name>`
- Delete: `DELETE /automata/v1/<name>`

### Mount/Unmount Autometa

- Mount: `POST /automata/v1/mount` - 在document上mount状态机,这个document会作为`RootDocument`被Automata接管
- Unmount: `POST /automata/v1/unmount` - 从document上unmount状态机

注意: Mount / Unmount 是对一个path pattern做,而不是单个独立的document,然后系统会自动匹配任何已经存在或者将来新创建的document,比如:

  **Mount**: users/{uid}/chats/{chat-id}
