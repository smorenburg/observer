rs.initiate({
   _id : "observer-db",
   members: [
      { _id: 0, host: "observer-db-0.observer-db.observer.svc.cluster.local:27017" },
      { _id: 1, host: "observer-db-1.observer-db.observer.svc.cluster.local:27017" },
      { _id: 2, host: "observer-db-2.observer-db.observer.svc.cluster.local:27017" }
   ]
})