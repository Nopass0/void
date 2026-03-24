import { VoidClient, query } from "../orm/typescript/dist/index.mjs";

const url = process.env.VOID_URL || "http://127.0.0.1:7700";
const username = process.env.VOID_USER || "admin";
const password = process.env.VOID_PASSWORD || "admin";
const dbName = process.env.VOID_DB || "seed_demo";
const colName = process.env.VOID_COLLECTION || "users";
const rows = Number(process.env.VOID_ROWS || "25");
const reset = process.env.VOID_RESET === "1";

async function ensureCollection(client) {
  const databases = await client.listDatabases();
  if (!databases.includes(dbName)) {
    await client.createDatabase(dbName);
  }

  const db = client.db(dbName);
  const collections = await db.listCollections();
  if (!collections.includes(colName)) {
    await db.createCollection(colName);
  }

  return db.collection(colName);
}

async function clearCollection(collection) {
  const docs = await collection.find(query().limit(500));
  for (const doc of docs) {
    await collection.delete(doc._id);
  }
  return docs.length;
}

async function main() {
  const client = new VoidClient({ url });
  await client.login(username, password);

  const users = await ensureCollection(client);

  let cleared = 0;
  if (reset) {
    cleared = await clearCollection(users);
  }

  const seededIds = [];
  for (let i = 0; i < rows; i += 1) {
    const id = await users.insert({
      name: `Seed User ${i + 1}`,
      age: 20 + (i % 25),
      active: i % 2 === 0,
      role: i % 5 === 0 ? "admin" : "user",
      score: 100 - i,
      created_from: "scripts/seed.mjs",
    });
    seededIds.push(id);
  }

  const admins = await users.find(
    query()
      .where("role", "eq", "admin")
      .where("active", "eq", true)
      .orderBy("score", "desc")
      .limit(10)
  );
  const total = await users.count();

  await client.cache.set(`seed:${dbName}:${colName}`, { total }, 120);
  const cached = await client.cache.get(`seed:${dbName}:${colName}`);

  console.log(
    JSON.stringify(
      {
        url,
        dbName,
        colName,
        reset,
        cleared,
        inserted: seededIds.length,
        firstId: seededIds[0] || null,
        adminMatches: admins.length,
        total,
        cached,
      },
      null,
      2
    )
  );
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
