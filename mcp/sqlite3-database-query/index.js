#!/usr/bin/env node

const path = require("path");
const fs = require("fs");
const BetterSqlite3 = require("better-sqlite3");
const { McpServer } = require("@modelcontextprotocol/sdk/server/mcp.js");
const { StdioServerTransport } = require("@modelcontextprotocol/sdk/server/stdio.js");
const { z } = require("zod");

// 连接缓存：<absPath, { db, lastAccess }>
const connectionCache = new Map();
const CACHE_MAX_SIZE = 5;

function getConnection(dbPath) {
  const absPath = path.resolve(dbPath);
  if (!fs.existsSync(absPath)) {
    throw new Error(`数据库文件不存在: ${absPath}`);
  }

  const cached = connectionCache.get(absPath);
  if (cached) {
    cached.lastAccess = Date.now();
    return cached.db;
  }

  // 淘汰最久未使用的连接
  if (connectionCache.size >= CACHE_MAX_SIZE) {
    let oldest = null;
    for (const [p, entry] of connectionCache) {
      if (!oldest || entry.lastAccess < oldest.lastAccess) {
        oldest = { path: p, lastAccess: entry.lastAccess };
      }
    }
    if (oldest) {
      connectionCache.get(oldest.path).db.close();
      connectionCache.delete(oldest.path);
    }
  }

  const db = new BetterSqlite3(absPath, { readonly: true });
  db.pragma("journal_mode = WAL");
  connectionCache.set(absPath, { db, lastAccess: Date.now() });
  return db;
}

const server = new McpServer({
  name: "secure-db-mcp",
  version: "1.0.0",
});

server.registerTool(
  "sqlite3-database-query",
  {
    description:
      "在sqlite3数据库中执行只读 SQL 查询 (SELECT)，请勿用于非sqlite3数据库。用于检索数据、统计信息等。严禁执行修改数据的操作",
    inputSchema: {
      sql: z.string().describe("SQL 查询语句，必须以 SELECT 开头。"),
      dbPath: z
        .string()
        .optional()
        .describe(
          '数据库文件的绝对路径。'
        ),
    },
  },
  async ({ sql, dbPath }) => {
    if (!sql.trim().toUpperCase().startsWith("SELECT")) {
      return {
        content: [{ type: "text", text: "错误：仅允许执行 SELECT 查询。" }],
        isError: true,
      };
    }

    try {
      const db = getConnection(dbPath);
      const stmt = db.prepare(sql);
      const rows = stmt.all();
      return {
        content: [
          {
            type: "text",
            text: `查询成功，返回 ${rows.length} 行:\n${JSON.stringify(rows, null, 2)}`,
          },
        ],
      };
    } catch (error) {
      return {
        content: [{ type: "text", text: `错误: ${error.message}` }],
        isError: true,
      };
    }
  }
);

const transport = new StdioServerTransport();
server.connect(transport);
